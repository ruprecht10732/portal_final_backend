package waagent

import (
	"context"
	"errors"
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
	errOrgContextUnavailable = "organization context not available"
)

// orgIDContextKey is used to inject org_id into tool.Context without exposing it to the LLM.
type orgIDContextKey struct{}
type phoneKeyContextKey struct{}

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
	tools, err := buildWhatsAppTools(toolHandler)
	if err != nil {
		return nil, err
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "WhatsAppAgent",
		Model:       kimi,
		Description: "Autonomous WhatsApp assistant for authenticated external users.",
		Instruction: workspace.Instruction,
		Toolsets:    orchestration.BuildWorkspaceToolsets(workspace, "whatsapp_agent_tools", tools),
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

func buildWhatsAppTools(toolHandler *ToolHandler) ([]tool.Tool, error) {
	searchLeadsTool, err := buildSearchLeadsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	getAvailableVisitSlotsTool, err := buildGetAvailableVisitSlotsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	getLeadDetailsTool, err := buildGetLeadDetailsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	getNavigationLinkTool, err := buildGetNavigationLinkTool(toolHandler)
	if err != nil {
		return nil, err
	}

	createLeadTool, err := buildCreateLeadTool(toolHandler)
	if err != nil {
		return nil, err
	}

	searchProductMaterialsTool, err := buildSearchProductMaterialsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	getQuotesTool, err := buildGetQuotesTool(toolHandler)
	if err != nil {
		return nil, err
	}

	getAppointmentsTool, err := buildGetAppointmentsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	updateLeadDetailsTool, err := buildUpdateLeadDetailsTool(toolHandler)
	if err != nil {
		return nil, err
	}

	askCustomerClarificationTool, err := buildAskCustomerClarificationTool(toolHandler)
	if err != nil {
		return nil, err
	}

	saveNoteTool, err := buildSaveNoteTool(toolHandler)
	if err != nil {
		return nil, err
	}

	updateStatusTool, err := buildUpdateStatusTool(toolHandler)
	if err != nil {
		return nil, err
	}

	scheduleVisitTool, err := buildScheduleVisitTool(toolHandler)
	if err != nil {
		return nil, err
	}

	rescheduleVisitTool, err := buildRescheduleVisitTool(toolHandler)
	if err != nil {
		return nil, err
	}

	cancelVisitTool, err := buildCancelVisitTool(toolHandler)
	if err != nil {
		return nil, err
	}

	return []tool.Tool{
		searchLeadsTool,
		createLeadTool,
		searchProductMaterialsTool,
		getAvailableVisitSlotsTool,
		getLeadDetailsTool,
		getNavigationLinkTool,
		getQuotesTool,
		getAppointmentsTool,
		updateLeadDetailsTool,
		askCustomerClarificationTool,
		saveNoteTool,
		updateStatusTool,
		scheduleVisitTool,
		rescheduleVisitTool,
		cancelVisitTool,
	}, nil
}

func buildGetLeadDetailsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	leadDetailsTool, err := apptools.NewGetLeadDetailsTool(func(ctx tool.Context, input GetLeadDetailsInput) (GetLeadDetailsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetLeadDetailsOutput{}, err
		}
		return toolHandler.HandleGetLeadDetails(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetLeadDetails tool: %w", err)
	}
	return leadDetailsTool, nil
}

func buildCreateLeadTool(toolHandler *ToolHandler) (tool.Tool, error) {
	createLeadTool, err := apptools.NewCreateLeadTool(func(ctx tool.Context, input CreateLeadInput) (CreateLeadOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return CreateLeadOutput{}, err
		}
		return toolHandler.HandleCreateLead(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build CreateLead tool: %w", err)
	}
	return createLeadTool, nil
}

func buildSearchProductMaterialsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	searchTool, err := apptools.NewSearchProductMaterialsTool(func(ctx tool.Context, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return SearchProductMaterialsOutput{}, err
		}
		return toolHandler.HandleSearchProductMaterials(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build SearchProductMaterials tool: %w", err)
	}
	return searchTool, nil
}

func buildGetNavigationLinkTool(toolHandler *ToolHandler) (tool.Tool, error) {
	navigationTool, err := apptools.NewGetNavigationLinkTool(func(ctx tool.Context, input GetNavigationLinkInput) (GetNavigationLinkOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetNavigationLinkOutput{}, err
		}
		return toolHandler.HandleGetNavigationLink(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetNavigationLink tool: %w", err)
	}
	return navigationTool, nil
}

func buildSearchLeadsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	searchLeadsTool, err := apptools.NewSearchLeadsTool(func(ctx tool.Context, input SearchLeadsInput) (SearchLeadsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return SearchLeadsOutput{}, err
		}
		return toolHandler.HandleSearchLeads(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build SearchLeads tool: %w", err)
	}
	return searchLeadsTool, nil
}

func buildGetAvailableVisitSlotsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	visitSlotsTool, err := apptools.NewGetAvailableVisitSlotsTool(func(ctx tool.Context, input GetAvailableVisitSlotsInput) (GetAvailableVisitSlotsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetAvailableVisitSlotsOutput{}, err
		}
		return toolHandler.HandleGetAvailableVisitSlots(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetAvailableVisitSlots tool: %w", err)
	}
	return visitSlotsTool, nil
}

func buildGetQuotesTool(toolHandler *ToolHandler) (tool.Tool, error) {
	quotesTool, err := apptools.NewGetQuotesTool(func(ctx tool.Context, input GetPendingQuotesInput) (GetPendingQuotesOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetPendingQuotesOutput{}, err
		}
		return toolHandler.HandleGetPendingQuotes(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetQuotes tool: %w", err)
	}
	return quotesTool, nil
}

func buildGetAppointmentsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	appointmentsTool, err := apptools.NewGetAppointmentsTool(func(ctx tool.Context, input GetAppointmentsInput) (GetAppointmentsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetAppointmentsOutput{}, err
		}
		return toolHandler.HandleGetAppointments(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build GetAppointments tool: %w", err)
	}
	return appointmentsTool, nil
}

func buildUpdateLeadDetailsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	leadDetailsTool, err := apptools.NewUpdateLeadDetailsTool("Updates lead contact or address details when the customer explicitly provides corrected information.", func(ctx tool.Context, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return UpdateLeadDetailsOutput{}, err
		}
		return toolHandler.HandleUpdateLeadDetails(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build UpdateLeadDetails tool: %w", err)
	}
	return leadDetailsTool, nil
}

func buildAskCustomerClarificationTool(toolHandler *ToolHandler) (tool.Tool, error) {
	clarificationTool, err := apptools.NewAskCustomerClarificationTool(func(ctx tool.Context, input AskCustomerClarificationInput) (AskCustomerClarificationOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return AskCustomerClarificationOutput{}, err
		}
		return toolHandler.HandleAskCustomerClarification(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build AskCustomerClarification tool: %w", err)
	}
	return clarificationTool, nil
}

func buildSaveNoteTool(toolHandler *ToolHandler) (tool.Tool, error) {
	noteTool, err := apptools.NewSaveNoteTool(func(ctx tool.Context, input SaveNoteInput) (SaveNoteOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return SaveNoteOutput{}, err
		}
		return toolHandler.HandleSaveNote(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build SaveNote tool: %w", err)
	}
	return noteTool, nil
}

func buildUpdateStatusTool(toolHandler *ToolHandler) (tool.Tool, error) {
	statusTool, err := apptools.NewUpdateStatusTool(func(ctx tool.Context, input UpdateStatusInput) (UpdateStatusOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return UpdateStatusOutput{}, err
		}
		return toolHandler.HandleUpdateStatus(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build UpdateStatus tool: %w", err)
	}
	return statusTool, nil
}

func buildScheduleVisitTool(toolHandler *ToolHandler) (tool.Tool, error) {
	scheduleTool, err := apptools.NewScheduleVisitTool(func(ctx tool.Context, input ScheduleVisitInput) (ScheduleVisitOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return ScheduleVisitOutput{}, err
		}
		return toolHandler.HandleScheduleVisit(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build ScheduleVisit tool: %w", err)
	}
	return scheduleTool, nil
}

func buildRescheduleVisitTool(toolHandler *ToolHandler) (tool.Tool, error) {
	rescheduleTool, err := apptools.NewRescheduleVisitTool(func(ctx tool.Context, input RescheduleVisitInput) (RescheduleVisitOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return RescheduleVisitOutput{}, err
		}
		return toolHandler.HandleRescheduleVisit(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build RescheduleVisit tool: %w", err)
	}
	return rescheduleTool, nil
}

func buildCancelVisitTool(toolHandler *ToolHandler) (tool.Tool, error) {
	cancelTool, err := apptools.NewCancelVisitTool(func(ctx tool.Context, input CancelVisitInput) (CancelVisitOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return CancelVisitOutput{}, err
		}
		return toolHandler.HandleCancelVisit(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("waagent: failed to build CancelVisit tool: %w", err)
	}
	return cancelTool, nil
}

func orgIDFromToolContext(ctx tool.Context) (uuid.UUID, error) {
	orgID, ok := ctx.Value(orgIDContextKey{}).(uuid.UUID)
	if !ok {
		return uuid.Nil, errors.New(errOrgContextUnavailable)
	}
	return orgID, nil
}

func phoneKeyFromToolContext(ctx tool.Context) (string, bool) {
	phoneKey, ok := ctx.Value(phoneKeyContextKey{}).(string)
	if !ok || strings.TrimSpace(phoneKey) == "" {
		return "", false
	}
	return strings.TrimSpace(phoneKey), true
}

// Run executes the agent with conversation history and returns the text reply.
func (a *Agent) Run(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint) (string, error) {
	// Inject org_id into context for tool handlers (never in the LLM prompt).
	ctx = context.WithValue(ctx, orgIDContextKey{}, orgID)
	ctx = context.WithValue(ctx, phoneKeyContextKey{}, strings.TrimSpace(phoneKey))

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

	parts := buildConversationParts(messages, leadHint)
	if len(parts) == 0 {
		return "", fmt.Errorf("waagent: no messages to process")
	}

	userMessage := &genai.Content{
		Role:  "user",
		Parts: parts,
	}

	return a.collectRunOutput(ctx, userID, sessionID, userMessage)
}

func buildConversationParts(messages []ConversationMessage, leadHint *ConversationLeadHint) []*genai.Part {
	parts := make([]*genai.Part, 0, len(messages)+1)
	if leadHint != nil && strings.TrimSpace(leadHint.LeadID) != "" {
		hintText := "[Gesprekshint]: Laatst opgeloste lead_id=" + leadHint.LeadID
		if strings.TrimSpace(leadHint.CustomerName) != "" {
			hintText += ", klant=" + leadHint.CustomerName
		}
		parts = append(parts, &genai.Part{Text: assistantPrefix + hintText})
	}
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
