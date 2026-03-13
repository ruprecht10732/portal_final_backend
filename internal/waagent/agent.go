package waagent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"portal_final_backend/internal/orchestration"
	apptools "portal_final_backend/internal/tools"
	"portal_final_backend/platform/ai/moonshot"
)

const (
	agentWorkspaceName = "whatsapp-agent"
	agentAppName       = "whatsapp-agent"
	maxToolIterations  = 10
	assistantPrefix    = "[Jouw vorig antwoord]: "
	userPrefix         = "[Klant]: "
)

// orgIDContextKey is used to inject org_id into tool.Context without exposing it to the LLM.
type orgIDContextKey struct{}

// ConversationMessage represents a single message in the conversation history.
type ConversationMessage struct {
	Role    string
	Content string
}

// Agent wraps the ADK agent and runner for the WhatsApp agent.
type Agent struct {
	adkAgent       agent.Agent
	runner         *runner.Runner
	sessionService session.Service
}

// NewAgent creates a new WhatsApp agent with function-calling tools.
func NewAgent(modelCfg moonshot.Config, toolHandler *ToolHandler) (*Agent, error) {
	workspace, err := orchestration.LoadAgentWorkspace(agentWorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to load workspace: %w", err)
	}

	kimi := moonshot.NewModel(modelCfg)

	getPendingQuotesTool, err := apptools.NewGetPendingQuotesTool(func(ctx tool.Context, input GetPendingQuotesInput) (GetPendingQuotesOutput, error) {
		orgID, ok := ctx.Value(orgIDContextKey{}).(uuid.UUID)
		if !ok {
			return GetPendingQuotesOutput{}, fmt.Errorf("organization context not available")
		}
		return toolHandler.HandleGetPendingQuotes(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetPendingQuotes tool: %w", err)
	}

	getAppointmentsTool, err := apptools.NewGetAppointmentsTool(func(ctx tool.Context, input GetAppointmentsInput) (GetAppointmentsOutput, error) {
		orgID, ok := ctx.Value(orgIDContextKey{}).(uuid.UUID)
		if !ok {
			return GetAppointmentsOutput{}, fmt.Errorf("organization context not available")
		}
		return toolHandler.HandleGetAppointments(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetAppointments tool: %w", err)
	}

	toolsets := orchestration.BuildWorkspaceToolsets(workspace, "whatsapp_agent_tools", []tool.Tool{getPendingQuotesTool, getAppointmentsTool})

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "WhatsAppAgent",
		Model:       kimi,
		Description: "Autonomous WhatsApp assistant for authenticated external users.",
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to create agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        agentAppName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to create runner: %w", err)
	}

	return &Agent{
		adkAgent:       adkAgent,
		runner:         r,
		sessionService: sessionService,
	}, nil
}

// Run executes the agent with conversation history and returns the text reply.
func (a *Agent) Run(ctx context.Context, orgID uuid.UUID, messages []ConversationMessage) (string, error) {
	// Inject org_id into context for tool handlers (never in the LLM prompt).
	ctx = context.WithValue(ctx, orgIDContextKey{}, orgID)

	sessionID := uuid.New().String()
	userID := "waagent-" + orgID.String()

	_, err := a.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   agentAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("waagent: create session: %w", err)
	}
	defer func() {
		_ = a.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   agentAppName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	parts := buildConversationParts(messages)
	if len(parts) == 0 {
		return "", fmt.Errorf("waagent: no messages to process")
	}

	userMessage := &genai.Content{
		Role:  "user",
		Parts: parts,
	}

	return a.collectRunOutput(ctx, userID, sessionID, userMessage)
}

func buildConversationParts(messages []ConversationMessage) []*genai.Part {
	parts := make([]*genai.Part, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, &genai.Part{Text: messagePrefix(msg.Role) + msg.Content})
	}
	return parts
}

func messagePrefix(role string) string {
	if role == "assistant" {
		return assistantPrefix
	}
	return userPrefix
}

func (a *Agent) collectRunOutput(ctx context.Context, userID, sessionID string, userMessage *genai.Content) (string, error) {
	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var outputText strings.Builder
	iterations := 0

	for event, err := range a.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		if err != nil {
			return "", fmt.Errorf("waagent: run failed: %w", err)
		}
		appendContentText(&outputText, event.Content)
		iterations++
		if iterations >= maxToolIterations {
			log.Printf("waagent: max iterations reached (%d), returning best-effort reply", maxToolIterations)
			break
		}
	}

	return strings.TrimSpace(outputText.String()), nil
}

func appendContentText(output *strings.Builder, content *genai.Content) {
	if content == nil {
		return
	}
	for _, part := range content.Parts {
		output.WriteString(part.Text)
	}
}
