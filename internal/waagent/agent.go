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
	"google.golang.org/adk/model"
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
	log.Printf("waagent: workspace loaded name=%s instruction_len=%d allowed_tools=%v",
		workspace.Name, len(workspace.Instruction), workspace.AllowedTools)

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
	log.Printf("waagent: Run org=%s phone=%s messages=%d hasLeadHint=%v", orgID, phoneKey, len(messages), leadHint != nil)
	// Inject org_id into context for tool handlers (never in the LLM prompt).
	ctx = context.WithValue(ctx, orgIDContextKey{}, orgID)
	ctx = context.WithValue(ctx, phoneKeyContextKey{}, strings.TrimSpace(phoneKey))

	sessionID := uuid.New().String()
	userID := "waagent-" + orgID.String()

	createResp, err := a.sessionService.Create(ctx, &session.CreateRequest{
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

	if len(messages) == 0 {
		return "", fmt.Errorf("waagent: no messages to process")
	}

	// Seed session with conversation history as proper multi-turn events
	// so the LLM sees real user/model turns instead of a flattened blob.
	historyMessages := messages[:len(messages)-1]
	latestMessage := messages[len(messages)-1]

	if err := a.seedSessionHistory(ctx, createResp.Session, historyMessages, leadHint); err != nil {
		log.Printf("waagent: failed to seed session history, falling back: %v", err)
	}

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: latestMessage.Content}},
	}

	return a.collectRunOutput(ctx, userID, sessionID, userMessage)
}

// seedSessionHistory populates the ADK session with prior conversation turns
// so the LLM receives proper multi-turn context. Also injects lead context
// as a synthetic assistant message if a lead hint is available.
func (a *Agent) seedSessionHistory(ctx context.Context, sess session.Session, history []ConversationMessage, leadHint *ConversationLeadHint) error {
	// If there's a lead hint, inject it as a system-context event at the start
	// so the model knows which lead the conversation is about.
	if leadHint != nil && strings.TrimSpace(leadHint.LeadID) != "" {
		hintText := a.buildLeadContextText(leadHint)
		hintEvent := session.NewEvent("history-hint")
		hintEvent.Author = "WhatsAppAgent"
		hintEvent.LLMResponse = model.LLMResponse{
			Content: genai.NewContentFromText(hintText, "model"),
		}
		if err := a.sessionService.AppendEvent(ctx, sess, hintEvent); err != nil {
			return fmt.Errorf("append lead hint event: %w", err)
		}
	}

	for i, msg := range history {
		event := session.NewEvent(fmt.Sprintf("history-%d", i))
		if msg.Role == "assistant" {
			event.Author = "WhatsAppAgent"
			event.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(msg.Content, "model"),
			}
		} else {
			event.Author = "user"
			event.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(msg.Content, "user"),
			}
		}
		if err := a.sessionService.AppendEvent(ctx, sess, event); err != nil {
			return fmt.Errorf("append history event %d: %w", i, err)
		}
	}
	return nil
}

// buildLeadContextText produces a rich context string from the lead hint.
// When preloaded details are available, the model gets address, contact info,
// service type, and status without needing an extra tool call.
func (a *Agent) buildLeadContextText(hint *ConversationLeadHint) string {
	if hint.PreloadedDetails != nil {
		return formatPreloadedDetails(hint.PreloadedDetails)
	}
	// Fallback: minimal hint
	text := "Gesprekcontext: laatst besproken klant"
	if strings.TrimSpace(hint.CustomerName) != "" {
		text += " " + hint.CustomerName
	}
	text += ". Gebruik GetLeadDetails als je meer gegevens nodig hebt over deze klant."
	return text
}

func formatPreloadedDetails(d *LeadDetailsResult) string {
	var b strings.Builder
	b.WriteString("Gesprekcontext voor deze klant:\n")
	if d.CustomerName != "" {
		b.WriteString("- Klant: " + d.CustomerName + "\n")
	}
	if d.FullAddress != "" {
		b.WriteString("- Adres: " + d.FullAddress + "\n")
	} else {
		addr := buildAddressLine(d.Street, d.HouseNumber, d.ZipCode, d.City)
		if addr != "" {
			b.WriteString("- Adres: " + addr + "\n")
		}
	}
	if d.Phone != "" {
		b.WriteString("- Telefoon: " + d.Phone + "\n")
	}
	if d.Email != "" {
		b.WriteString("- E-mail: " + d.Email + "\n")
	}
	if d.ServiceType != "" {
		b.WriteString("- Dienst: " + d.ServiceType + "\n")
	}
	if d.Status != "" {
		b.WriteString("- Status: " + d.Status + "\n")
	}
	b.WriteString("(Deze gegevens komen uit het CRM. Gebruik GetLeadDetails voor de meest actuele gegevens als je twijfelt.)")
	return b.String()
}

func buildAddressLine(street, houseNumber, zipCode, city string) string {
	var parts []string
	streetPart := strings.TrimSpace(street)
	if hn := strings.TrimSpace(houseNumber); hn != "" {
		streetPart += " " + hn
	}
	if strings.TrimSpace(streetPart) != "" {
		parts = append(parts, strings.TrimSpace(streetPart))
	}
	if zc := strings.TrimSpace(zipCode); zc != "" {
		parts = append(parts, zc)
	}
	if c := strings.TrimSpace(city); c != "" {
		parts = append(parts, c)
	}
	return strings.Join(parts, ", ")
}

func (a *Agent) collectRunOutput(ctx context.Context, userID, sessionID string, userMessage *genai.Content) (string, error) {
	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var lastFinalText string
	iterations := 0

	for event, err := range a.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		if err != nil {
			return "", fmt.Errorf("waagent: run failed: %w", err)
		}
		// Only keep text from the final response event — intermediate
		// tool-thinking events produce disjointed fragments that get
		// concatenated into garbled output.
		if event.IsFinalResponse() {
			if text := extractContentText(event.Content); text != "" {
				lastFinalText = text
			}
		}
		iterations++
		if iterations >= maxToolIterations {
			log.Printf("waagent: max iterations reached (%d), returning best-effort reply", maxToolIterations)
			break
		}
	}

	return strings.TrimSpace(lastFinalText), nil
}

func extractContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range content.Parts {
		b.WriteString(part.Text)
	}
	return b.String()
}
