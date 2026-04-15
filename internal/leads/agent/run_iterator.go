package agent

import (
	"context"
	"fmt"
	"iter"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/repository"
)

const maxToolCallsPerSession = 30

type observedToolTrace struct {
	Kind     string
	Name     string
	ID       string
	Keys     []string
	HasError bool
}

func consumeRunEvents[T any](seq iter.Seq2[T, error], runFailureMessage string, handle func(T), observers ...func(T)) error {
	for event, err := range seq {
		if err != nil {
			return fmt.Errorf("%s: %w", runFailureMessage, err)
		}
		if handle != nil {
			handle(event)
		}
		for _, observer := range observers {
			if observer != nil {
				observer(event)
			}
		}
	}

	return nil
}

type promptRunRequest struct {
	SessionService       session.Service
	Runner               *runner.Runner
	AppName              string
	UserID               string
	SessionID            string
	UserMessage          *genai.Content
	CreateSessionMessage string
	RunFailureMessage    string
	TraceLabel           string
	// SkipSessionLifecycle skips session creation and deletion when the
	// caller manages the session lifecycle externally (e.g. photo analyzer
	// that reuses a session across analysis + retry).
	SkipSessionLifecycle bool
	// OnSessionComplete is an optional callback invoked at the end of a
	// session with the accumulated run metrics. Callers that persist
	// agent_runs records use this to capture tool-call counts, durations,
	// and trace data without changing the return signature.
	OnSessionComplete func(SessionResult)
}

// SessionResult captures per-session metrics collected during runPromptSession.
type SessionResult struct {
	ToolCallCount int
	DurationMs    int
	ToolTraces    []observedToolTrace
}

func runPromptSession(ctx context.Context, req promptRunRequest, handle func(*session.Event)) error {
	sessionStart := time.Now()
	if !req.SkipSessionLifecycle {
		_, err := req.SessionService.Create(ctx, &session.CreateRequest{
			AppName:   req.AppName,
			UserID:    req.UserID,
			SessionID: req.SessionID,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", req.CreateSessionMessage, err)
		}
		defer func() {
			_ = req.SessionService.Delete(ctx, &session.DeleteRequest{
				AppName:   req.AppName,
				UserID:    req.UserID,
				SessionID: req.SessionID,
			})
		}()
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var toolTrace []observedToolTrace
	toolCallCount := 0
	budgetCtx, budgetCancel := context.WithCancel(ctx)
	defer budgetCancel()
	enforceToolCallLimit := func(event *session.Event) {
		if event == nil || event.Content == nil {
			return
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil {
				toolCallCount++
			}
		}
		if toolCallCount >= maxToolCallsPerSession {
			log.Printf("%s: cancelling session at %d tool calls (limit %d) user=%s session=%s",
				req.TraceLabel, toolCallCount, maxToolCallsPerSession, req.UserID, req.SessionID)
			budgetCancel()
		}
	}
	err := consumeRunEvents(req.Runner.Run(budgetCtx, req.UserID, req.SessionID, req.UserMessage, runConfig), req.RunFailureMessage, handle, observeSessionToolTrace(&toolTrace), enforceToolCallLimit)
	logObservedToolTrace(req.TraceLabel, req.UserID, req.SessionID, toolTrace)

	durationMs := int(time.Since(sessionStart).Milliseconds())
	if req.OnSessionComplete != nil {
		req.OnSessionComplete(SessionResult{
			ToolCallCount: toolCallCount,
			DurationMs:    durationMs,
			ToolTraces:    toolTrace,
		})
	}

	if err != nil && toolCallCount >= maxToolCallsPerSession {
		log.Printf("%s: session aborted after %d tool calls (limit %d) user=%s session=%s",
			req.TraceLabel, toolCallCount, maxToolCallsPerSession, req.UserID, req.SessionID)
		return fmt.Errorf("%s: tool call limit exceeded (%d >= %d)", req.TraceLabel, toolCallCount, maxToolCallsPerSession)
	}
	return err
}

func runPromptTextSession(ctx context.Context, req promptRunRequest, promptText string) (string, error) {
	var output strings.Builder
	req.UserMessage = &genai.Content{Role: "user", Parts: []*genai.Part{{Text: promptText}}}
	err := runPromptSession(ctx, req, func(event *session.Event) {
		output.WriteString(collectContentText(event.Content))
	})
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

func observeSessionToolTrace(items *[]observedToolTrace) func(*session.Event) {
	return func(event *session.Event) {
		if items == nil {
			return
		}
		*items = append(*items, extractSessionToolTrace(event)...)
	}
}

func extractSessionToolTrace(event *session.Event) []observedToolTrace {
	if event == nil || event.Content == nil {
		return nil
	}
	traces := make([]observedToolTrace, 0, len(event.Content.Parts))
	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if call := part.FunctionCall; call != nil {
			traces = append(traces, observedToolTrace{
				Kind: "call",
				Name: strings.TrimSpace(call.Name),
				ID:   strings.TrimSpace(call.ID),
				Keys: sortedMapKeys(call.Args),
			})
		}
		if response := part.FunctionResponse; response != nil {
			traces = append(traces, observedToolTrace{
				Kind:     "response",
				Name:     strings.TrimSpace(response.Name),
				ID:       strings.TrimSpace(response.ID),
				Keys:     sortedMapKeys(response.Response),
				HasError: hasResponseError(response),
			})
		}
	}
	return traces
}

func logObservedToolTrace(agentName, userID, sessionID string, traces []observedToolTrace) {
	if len(traces) == 0 {
		return
	}
	items := make([]string, 0, len(traces))
	for _, trace := range traces {
		items = append(items, formatObservedToolTrace(trace))
	}
	if len(items) > 12 {
		items = append(items[:12], fmt.Sprintf("...+%d more", len(items)-12))
	}
	log.Printf("%s: tool trace user=%s session=%s count=%d sequence=%s", agentName, userID, sessionID, len(traces), strings.Join(items, " | "))
}

func formatObservedToolTrace(trace observedToolTrace) string {
	name := trace.Name
	if name == "" {
		name = "unknown"
	}
	keys := "none"
	if len(trace.Keys) > 0 {
		keys = strings.Join(limitTraceKeys(trace.Keys), ",")
	}
	status := "ok"
	if trace.HasError {
		status = "error"
	}
	if trace.Kind == "call" {
		return fmt.Sprintf("call:%s(keys=%s)", name, keys)
	}
	return fmt.Sprintf("response:%s(status=%s,keys=%s)", name, status, keys)
}

func sortedMapKeys(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func limitTraceKeys(keys []string) []string {
	if len(keys) <= 4 {
		return keys
	}
	limited := append([]string(nil), keys[:4]...)
	return append(limited, "...")
}

func hasResponseError(response *genai.FunctionResponse) bool {
	if response == nil || len(response.Response) == 0 {
		return false
	}
	_, ok := response.Response["error"]
	return ok
}

// persistToolTraces writes the observed tool traces of a session to the
// agent_tool_calls table so that every individual tool invocation is
// queryable later. It is fire-and-forget; errors are logged but do not
// propagate.
func persistToolTraces(ctx context.Context, repo interface {
	InsertAgentToolCall(ctx context.Context, params repository.InsertAgentToolCallParams) error
}, agentRunID uuid.UUID, traces []observedToolTrace, label string) {
	if len(traces) == 0 || agentRunID == uuid.Nil {
		return
	}
	seq := 0
	for _, t := range traces {
		if t.Kind != "call" && t.Kind != "response" {
			continue
		}
		seq++
		params := repository.InsertAgentToolCallParams{
			AgentRunID:  agentRunID,
			SequenceNum: seq,
			ToolName:    t.Name,
			HasError:    t.HasError,
		}
		if err := repo.InsertAgentToolCall(ctx, params); err != nil {
			log.Printf("%s: failed to persist tool trace seq=%d tool=%s: %v", label, seq, t.Name, err)
		}
	}
}
