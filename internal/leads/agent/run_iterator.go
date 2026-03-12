package agent

import (
	"fmt"
	"iter"
	"log"
	"sort"
	"strings"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

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
