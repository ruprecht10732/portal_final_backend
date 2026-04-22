package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/logger"
)

const (
	agentWorkspaceName        = "whatsapp-agent"
	agentPartnerWorkspaceName = "whatsapp_partner_agent"
	agentAppName              = "whatsapp-agent"
	agentPartnerAppName       = "whatsapp_partner_agent"
	maxToolIterations         = 10
	assistantPrefix           = "[Jouw vorig antwoord]: "
	userPrefix                = "[Klant]: "
	errOrgContextUnavailable  = "organization context not available"
)

type agentRunMode string

const (
	agentRunModeDefault agentRunMode = "default"
	agentRunModePartner agentRunMode = "partner"
)

// orgIDContextKey is used to inject org_id into tool.Context without exposing it to the LLM.
type orgIDContextKey struct{}
type phoneKeyContextKey struct{}
type partnerIDContextKey struct{}
type currentInboundMessageContextKey struct{}

// ConversationMessage represents a single message in the conversation history.
type ConversationMessage struct {
	Role    string
	Content string
	SentAt  *time.Time
}

// Agent wraps the ADK agent and runner for the WhatsApp agent.
type Agent struct {
	defaultRuntime agentRuntime
	partnerRuntime agentRuntime
	sessionService session.Service
	log            *logger.Logger
}

type agentRuntime struct {
	appName string
	runner  *runner.Runner
}

type agentRuntimeConfig struct {
	appName     string
	agentName   string
	instruction string
	toolBuilder func(*ToolHandler) ([]tool.Tool, error)
}

type agentExecutionRequest struct {
	orgID          uuid.UUID
	phoneKey       string
	messages       []ConversationMessage
	leadHint       *ConversationLeadHint
	inboundMessage *CurrentInboundMessage
}

type AgentRunResult struct {
	Reply             string
	ToolResponseNames []string
	ToolResponseCount int
}

type toolResponseObservation struct {
	Name    string
	Payload string
}

type replyGroundingEvidence struct {
	toolResponseNames map[string]int
	toolResponses     []toolResponseObservation
}

type groundingDecision struct {
	Code             string
	UnsupportedFacts []string
}

var (
	reCurrencyAmount = regexp.MustCompile(`€\s?[0-9][0-9.]*([,][0-9]{2})?`)
	reClockTime      = regexp.MustCompile(`\b\d{1,2}:\d{2}\b`)
	reISODate        = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	reNumericDate    = regexp.MustCompile(`\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b`)
	reDutchDate      = regexp.MustCompile(`(?i)\b\d{1,2}\s+(januari|februari|maart|april|mei|juni|juli|augustus|september|oktober|november|december)\s+\d{4}\b`)
	reEmail          = regexp.MustCompile(`(?i)\b[\w.%+-]+@[\w.-]+\.[a-z]{2,}\b`)
	rePhone          = regexp.MustCompile(`(?:\+31|0)[0-9\s\-()]{8,}`)
	reAddressLine    = regexp.MustCompile(`(?im)\badres:\s*([^\n]+)`)
	reQuoteLine      = regexp.MustCompile(`(?im)\b(?:offerte|offertenummer|quotenummer):\s*([^\n]+)`)
	reStatusLine     = regexp.MustCompile(`(?im)\bstatus:\s*([^\n]+)`)
	reQuoteNumber    = regexp.MustCompile(`(?i)^[a-z]{2,}[a-z0-9-]*\d[a-z0-9-]*$`)
)

var appointmentContextKeywords = []string{
	"afspraak",
	"afspraken",
	"bezoek",
	"bezoeken",
	"ingepland",
	"inplannen",
	"monteur",
	"tijdvak",
	"inspectie",
	"opname",
}

var dutchMonthNumbers = map[string]string{
	"januari":   "01",
	"februari":  "02",
	"maart":     "03",
	"april":     "04",
	"mei":       "05",
	"juni":      "06",
	"juli":      "07",
	"augustus":  "08",
	"september": "09",
	"oktober":   "10",
	"november":  "11",
	"december":  "12",
}

// NewAgent creates a new WhatsApp agent with function-calling tools.
func NewAgent(modelCfg openaicompat.Config, toolHandler *ToolHandler, log *logger.Logger) (*Agent, error) {
	workspace, err := orchestration.LoadAgentWorkspace(agentWorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to load workspace: %w", err)
	}
	if log != nil {
		log.Info("whatsappagent: workspace loaded", "name", workspace.Name, "instruction_length", len(workspace.Instruction), "allowed_tools", workspace.AllowedTools)
	}

	sessionService := session.InMemoryService()
	defaultRuntime, err := newAgentRuntime(modelCfg, workspace, sessionService, toolHandler, agentRuntimeConfig{
		appName:     agentAppName,
		agentName:   "WhatsAppAgent",
		instruction: workspace.Instruction,
		toolBuilder: buildWhatsAppTools,
	})
	if err != nil {
		return nil, err
	}
	partnerWorkspace, err := orchestration.LoadAgentWorkspace(agentPartnerWorkspaceName)
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to load partner workspace: %w", err)
	}
	partnerRuntime, err := newAgentRuntime(modelCfg, partnerWorkspace, sessionService, toolHandler, agentRuntimeConfig{
		appName:     agentPartnerAppName,
		agentName:   "WhatsAppPartnerAgent",
		instruction: partnerWorkspace.Instruction,
		toolBuilder: buildPartnerWhatsAppTools,
	})
	if err != nil {
		return nil, err
	}
	return &Agent{
		defaultRuntime: defaultRuntime,
		partnerRuntime: partnerRuntime,
		sessionService: sessionService,
		log:            log,
	}, nil
}

func newAgentRuntime(modelCfg openaicompat.Config, workspace orchestration.Workspace, sessionService session.Service, toolHandler *ToolHandler, cfg agentRuntimeConfig) (agentRuntime, error) {
	kimi := openaicompat.NewModel(modelCfg)
	tools, err := cfg.toolBuilder(toolHandler)
	if err != nil {
		return agentRuntime{}, err
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        cfg.agentName,
		Model:       kimi,
		Description: "Autonomous WhatsApp assistant for authenticated external users.",
		Instruction: cfg.instruction,
		Toolsets:    orchestration.BuildWorkspaceToolsets(workspace, cfg.appName+"_tools", tools),
	})
	if err != nil {
		return agentRuntime{}, fmt.Errorf("whatsappagent: failed to create agent runtime %s: %w", cfg.appName, err)
	}
	r, err := runner.New(runner.Config{
		AppName:        cfg.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return agentRuntime{}, fmt.Errorf("whatsappagent: failed to create runner %s: %w", cfg.appName, err)
	}
	return agentRuntime{appName: cfg.appName, runner: r}, nil
}

func buildWhatsAppTools(toolHandler *ToolHandler) ([]tool.Tool, error) {
	return buildTools(toolHandler,
		buildSearchLeadsTool,
		buildCreateLeadTool,
		buildSearchProductMaterialsTool,
		buildAttachCurrentWhatsAppPhotoTool,
		buildGetAvailableVisitSlotsTool,
		buildGetLeadDetailsTool,
		buildGetEnergyLabelTool,
		buildGetLeadTasksTool,
		buildGetISDETool,
		buildGetNavigationLinkTool,
		buildGetQuotesTool,
		buildDraftQuoteTool,
		buildGenerateQuoteTool,
		buildSendQuotePDFTool,
		buildGetAppointmentsTool,
		buildCreateTaskTool,
		buildUpdateLeadDetailsTool,
		buildAskCustomerClarificationTool,
		buildSaveNoteTool,
		buildUpdateStatusTool,
		buildScheduleVisitTool,
		buildRescheduleVisitTool,
		buildCancelVisitTool,
	)
}

func buildPartnerWhatsAppTools(toolHandler *ToolHandler) ([]tool.Tool, error) {
	return buildTools(toolHandler,
		buildGetMyJobsTool,
		buildGetPartnerJobDetailsTool,
		buildGetNavigationLinkTool,
		buildGetAppointmentsTool,
		buildAttachCurrentWhatsAppPhotoTool,
		buildSaveMeasurementTool,
		buildUpdateAppointmentStatusTool,
		buildRescheduleVisitTool,
		buildCancelVisitTool,
		buildSaveNoteTool,
		buildSearchProductMaterialsTool,
	)
}

func buildTools(toolHandler *ToolHandler, builders ...func(*ToolHandler) (tool.Tool, error)) ([]tool.Tool, error) {
	tools := make([]tool.Tool, 0, len(builders))
	for _, builder := range builders {
		builtTool, err := builder(toolHandler)
		if err != nil {
			return nil, err
		}
		tools = append(tools, builtTool)
	}
	return tools, nil
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
		return nil, fmt.Errorf("whatsappagent: failed to build GetLeadDetails tool: %w", err)
	}
	return leadDetailsTool, nil
}

func buildGetEnergyLabelTool(toolHandler *ToolHandler) (tool.Tool, error) {
	energyLabelTool, err := apptools.NewGetEnergyLabelTool(func(ctx tool.Context, input GetEnergyLabelInput) (GetEnergyLabelOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetEnergyLabelOutput{}, err
		}
		return toolHandler.HandleGetEnergyLabel(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetEnergyLabel tool: %w", err)
	}
	return energyLabelTool, nil
}

func buildGetLeadTasksTool(toolHandler *ToolHandler) (tool.Tool, error) {
	leadTasksTool, err := apptools.NewGetLeadTasksTool(func(ctx tool.Context, input GetLeadTasksInput) (GetLeadTasksOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetLeadTasksOutput{}, err
		}
		return toolHandler.HandleGetLeadTasks(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetLeadTasks tool: %w", err)
	}
	return leadTasksTool, nil
}

func buildGetISDETool(toolHandler *ToolHandler) (tool.Tool, error) {
	isdeTool, err := apptools.NewGetISDETool(func(ctx tool.Context, input GetISDEInput) (GetISDEOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetISDEOutput{}, err
		}
		return toolHandler.HandleGetISDE(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetISDE tool: %w", err)
	}
	return isdeTool, nil
}

func buildGetMyJobsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	jobsTool, err := apptools.NewGetMyJobsTool(func(ctx tool.Context, input GetMyJobsInput) (GetMyJobsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetMyJobsOutput{}, err
		}
		return toolHandler.HandleGetMyJobs(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetMyJobs tool: %w", err)
	}
	return jobsTool, nil
}

func buildGetPartnerJobDetailsTool(toolHandler *ToolHandler) (tool.Tool, error) {
	detailsTool, err := apptools.NewGetPartnerJobDetailsTool(func(ctx tool.Context, input GetPartnerJobDetailsInput) (GetPartnerJobDetailsOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetPartnerJobDetailsOutput{}, err
		}
		return toolHandler.HandleGetPartnerJobDetails(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetPartnerJobDetails tool: %w", err)
	}
	return detailsTool, nil
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
		return nil, fmt.Errorf("whatsappagent: failed to build CreateLead tool: %w", err)
	}
	return createLeadTool, nil
}

func buildCreateTaskTool(toolHandler *ToolHandler) (tool.Tool, error) {
	createTaskTool, err := apptools.NewCreateTaskTool(func(ctx tool.Context, input CreateTaskInput) (CreateTaskOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return CreateTaskOutput{}, err
		}
		return toolHandler.HandleCreateTask(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build CreateTask tool: %w", err)
	}
	return createTaskTool, nil
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
		return nil, fmt.Errorf("whatsappagent: failed to build SearchProductMaterials tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build GetNavigationLink tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build SearchLeads tool: %w", err)
	}
	return searchLeadsTool, nil
}

func buildAttachCurrentWhatsAppPhotoTool(toolHandler *ToolHandler) (tool.Tool, error) {
	attachTool, err := apptools.NewAttachCurrentWhatsAppPhotoTool(func(ctx tool.Context, input AttachCurrentWhatsAppPhotoInput) (AttachCurrentWhatsAppPhotoOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return AttachCurrentWhatsAppPhotoOutput{}, err
		}
		return toolHandler.HandleAttachCurrentWhatsAppPhoto(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build AttachCurrentWhatsAppPhoto tool: %w", err)
	}
	return attachTool, nil
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
		return nil, fmt.Errorf("whatsappagent: failed to build GetAvailableVisitSlots tool: %w", err)
	}
	return visitSlotsTool, nil
}

func buildGetQuotesTool(toolHandler *ToolHandler) (tool.Tool, error) {
	quotesTool, err := apptools.NewGetQuotesTool(func(ctx tool.Context, input GetQuotesInput) (GetQuotesOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GetQuotesOutput{}, err
		}
		return toolHandler.HandleGetQuotes(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GetQuotes tool: %w", err)
	}
	return quotesTool, nil
}

func buildDraftQuoteTool(toolHandler *ToolHandler) (tool.Tool, error) {
	draftTool, err := apptools.NewDraftQuoteTool(func(ctx tool.Context, input DraftQuoteInput) (DraftQuoteOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return DraftQuoteOutput{}, err
		}
		return toolHandler.HandleDraftQuote(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build DraftQuote tool: %w", err)
	}
	return draftTool, nil
}

func buildGenerateQuoteTool(toolHandler *ToolHandler) (tool.Tool, error) {
	generateTool, err := apptools.NewGenerateQuoteTool(func(ctx tool.Context, input GenerateQuoteInput) (GenerateQuoteOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return GenerateQuoteOutput{}, err
		}
		return toolHandler.HandleGenerateQuote(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build GenerateQuote tool: %w", err)
	}
	return generateTool, nil
}

func buildSendQuotePDFTool(toolHandler *ToolHandler) (tool.Tool, error) {
	sendTool, err := apptools.NewSendQuotePDFTool(func(ctx tool.Context, input SendQuotePDFInput) (SendQuotePDFOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return SendQuotePDFOutput{}, err
		}
		return toolHandler.HandleSendQuotePDF(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build SendQuotePDF tool: %w", err)
	}
	return sendTool, nil
}

func buildSaveMeasurementTool(toolHandler *ToolHandler) (tool.Tool, error) {
	saveTool, err := apptools.NewSaveMeasurementTool(func(ctx tool.Context, input SaveMeasurementInput) (SaveMeasurementOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return SaveMeasurementOutput{}, err
		}
		return toolHandler.HandleSaveMeasurement(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build SaveMeasurement tool: %w", err)
	}
	return saveTool, nil
}

func buildUpdateAppointmentStatusTool(toolHandler *ToolHandler) (tool.Tool, error) {
	statusTool, err := apptools.NewUpdateAppointmentStatusTool(func(ctx tool.Context, input UpdateAppointmentStatusInput) (UpdateAppointmentStatusOutput, error) {
		orgID, err := orgIDFromToolContext(ctx)
		if err != nil {
			return UpdateAppointmentStatusOutput{}, err
		}
		return toolHandler.HandleUpdateAppointmentStatus(ctx, orgID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("whatsappagent: failed to build UpdateAppointmentStatus tool: %w", err)
	}
	return statusTool, nil
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
		return nil, fmt.Errorf("whatsappagent: failed to build GetAppointments tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build UpdateLeadDetails tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build AskCustomerClarification tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build SaveNote tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build UpdateStatus tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build ScheduleVisit tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build RescheduleVisit tool: %w", err)
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
		return nil, fmt.Errorf("whatsappagent: failed to build CancelVisit tool: %w", err)
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

func partnerIDFromToolContext(ctx tool.Context) (uuid.UUID, bool) {
	partnerID, ok := ctx.Value(partnerIDContextKey{}).(uuid.UUID)
	if !ok || partnerID == uuid.Nil {
		return uuid.Nil, false
	}
	return partnerID, true
}

func currentInboundMessageFromToolContext(ctx tool.Context) (CurrentInboundMessage, bool) {
	message, ok := ctx.Value(currentInboundMessageContextKey{}).(CurrentInboundMessage)
	if !ok || strings.TrimSpace(message.ExternalMessageID) == "" {
		return CurrentInboundMessage{}, false
	}
	return message, true
}

// Run executes the agent with conversation history and returns the text reply.
func (a *Agent) Run(ctx context.Context, orgID uuid.UUID, phoneKey string, messages []ConversationMessage, leadHint *ConversationLeadHint, inboundMessage *CurrentInboundMessage, mode agentRunMode) (AgentRunResult, error) {
	a.logInfo(ctx, "whatsappagent: run started", "organization_id", orgID.String(), "phone", phoneKey, "messages", len(messages), "has_lead_hint", leadHint != nil)
	if len(messages) == 0 {
		return AgentRunResult{}, fmt.Errorf("whatsappagent: no messages to process")
	}
	ctx = a.enrichRunContext(ctx, orgID, phoneKey, inboundMessage)
	runtime := a.defaultRuntime
	if mode == agentRunModePartner {
		runtime = a.partnerRuntime
	}
	result, err := a.runExecutionPipeline(ctx, runtime, agentExecutionRequest{orgID: orgID, phoneKey: phoneKey, messages: messages, leadHint: leadHint, inboundMessage: inboundMessage})
	if err != nil {
		return AgentRunResult{}, err
	}
	return result, nil
}

func (a *Agent) enrichRunContext(ctx context.Context, orgID uuid.UUID, phoneKey string, inboundMessage *CurrentInboundMessage) context.Context {
	ctx = context.WithValue(ctx, orgIDContextKey{}, orgID)
	ctx = context.WithValue(ctx, phoneKeyContextKey{}, strings.TrimSpace(phoneKey))
	if inboundMessage != nil {
		ctx = context.WithValue(ctx, currentInboundMessageContextKey{}, *inboundMessage)
	}
	return ctx
}

func (a *Agent) runExecutionPipeline(ctx context.Context, runtime agentRuntime, request agentExecutionRequest) (AgentRunResult, error) {
	return a.runRuntimeConversation(ctx, runtime, request)
}

func (a *Agent) runRuntimeConversation(ctx context.Context, runtime agentRuntime, request agentExecutionRequest) (AgentRunResult, error) {
	sessionID := uuid.New().String()
	userID := "whatsappagent-" + request.orgID.String()
	createResp, err := a.sessionService.Create(ctx, &session.CreateRequest{AppName: runtime.appName, UserID: userID, SessionID: sessionID})
	if err != nil {
		return AgentRunResult{}, fmt.Errorf("whatsappagent: create session: %w", err)
	}
	defer func() {
		_ = a.sessionService.Delete(ctx, &session.DeleteRequest{AppName: runtime.appName, UserID: userID, SessionID: sessionID})
	}()
	historyMessages := request.messages[:len(request.messages)-1]
	latestMessage := request.messages[len(request.messages)-1]
	if err := a.seedSessionHistory(ctx, createResp.Session, historyMessages, request.leadHint); err != nil {
		a.logWarn(ctx, "whatsappagent: failed to seed session history; continuing without seeded history", "error", err)
	}
	userMessage := &genai.Content{Role: "user", Parts: []*genai.Part{{Text: latestMessage.Content}}}
	return a.collectRunOutput(ctx, runtime, userID, sessionID, userMessage)
}

// seedSessionHistory populates the ADK session with prior conversation turns
// so the LLM receives proper multi-turn context. It also injects a lead-routing
// hint when a lead was previously resolved for the conversation.
func (a *Agent) seedSessionHistory(ctx context.Context, sess session.Session, history []ConversationMessage, leadHint *ConversationLeadHint) error {
	// If there's a lead hint, inject it before history so the model knows which
	// customer the conversation most likely refers to. The hint is deliberately
	// phrased as routing context only and must not be treated as verified output.
	if hasConversationRoutingContext(leadHint) {
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
		formattedContent := formatConversationHistoryContent(msg)
		if msg.Role == "assistant" {
			event.Author = "WhatsAppAgent"
			event.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(formattedContent, "model"),
			}
		} else {
			event.Author = "user"
			event.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(formattedContent, "user"),
			}
		}
		if err := a.sessionService.AppendEvent(ctx, sess, event); err != nil {
			return fmt.Errorf("append history event %d: %w", i, err)
		}
	}
	return nil
}

func formatConversationHistoryContent(msg ConversationMessage) string {
	content := strings.TrimSpace(msg.Content)
	if msg.SentAt == nil || msg.SentAt.IsZero() {
		return content
	}
	return fmt.Sprintf("[Berichttijd: %s]\n%s", msg.SentAt.UTC().Format(time.RFC3339), content)
}

// buildLeadContextText produces a routing hint from the lead context.
// It intentionally avoids exposing concrete customer details as verified facts
// so the model still has to call tools for customer-facing specifics.
func (a *Agent) buildLeadContextText(hint *ConversationLeadHint) string {
	var b strings.Builder
	b.WriteString("Gesprekcontext: gebruik deze leadhint alleen om de juiste klant te herkennen. ")
	if hint.PreloadedDetails != nil && strings.TrimSpace(hint.CustomerName) == "" {
		hint.CustomerName = strings.TrimSpace(hint.PreloadedDetails.CustomerName)
	}
	if strings.TrimSpace(hint.CustomerName) != "" {
		b.WriteString("Laatst besproken klant: ")
		b.WriteString(hint.CustomerName)
		b.WriteString(". ")
	} else {
		b.WriteString("Er is een eerder opgeloste klant in dit gesprek. ")
	}
	if len(hint.RecentQuotes) > 0 {
		b.WriteString("Laatst getoonde offertes in dit gesprek: ")
		b.WriteString(formatRecentQuoteHints(hint.RecentQuotes))
		b.WriteString(". ")
	}
	if len(hint.RecentAppointments) > 0 {
		b.WriteString("Laatst getoonde afspraken in dit gesprek: ")
		b.WriteString(formatRecentAppointmentHints(hint.RecentAppointments))
		b.WriteString(". ")
	}
	if strings.TrimSpace(hint.LeadServiceID) != "" {
		b.WriteString("Er is ook al een dienstcontext gekoppeld aan dit gesprek. ")
	}
	b.WriteString("Gebruik deze context om sneller de juiste klant of het juiste dossier te herkennen, en verifieer concrete klant-, offerte- of afspraakdetails met GetLeadDetails, GetQuotes of GetAppointments zodra dat nodig is voor je antwoord.")
	return b.String()
}

func formatRecentQuoteHints(quotes []RecentQuoteHint) string {
	parts := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		fragment := strings.TrimSpace(quote.ClientName)
		if fragment == "" {
			fragment = strings.TrimSpace(quote.QuoteNumber)
		}
		if strings.TrimSpace(quote.QuoteNumber) != "" && !strings.Contains(fragment, quote.QuoteNumber) {
			fragment = strings.TrimSpace(fragment + " (" + quote.QuoteNumber + ")")
		}
		if strings.TrimSpace(quote.Summary) != "" {
			fragment = strings.TrimSpace(fragment + ": " + quote.Summary)
		}
		if fragment != "" {
			parts = append(parts, fragment)
		}
	}
	return strings.Join(parts, "; ")
}

func formatRecentAppointmentHints(appointments []RecentAppointmentHint) string {
	parts := make([]string, 0, len(appointments))
	for _, appointment := range appointments {
		fragment := strings.TrimSpace(appointment.Title)
		if fragment == "" {
			fragment = "afspraak"
		}
		if strings.TrimSpace(appointment.StartTime) != "" {
			fragment = strings.TrimSpace(fragment + " op " + appointment.StartTime)
		}
		if strings.TrimSpace(appointment.Location) != "" {
			fragment = strings.TrimSpace(fragment + " in " + appointment.Location)
		}
		parts = append(parts, fragment)
	}
	return strings.Join(parts, "; ")
}

func (a *Agent) collectRunOutput(ctx context.Context, runtime agentRuntime, userID, sessionID string, userMessage *genai.Content) (AgentRunResult, error) {
	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var lastFinalText string
	evidence := newReplyGroundingEvidence()

	for event, err := range runtime.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		if err != nil {
			return AgentRunResult{}, fmt.Errorf("whatsappagent: run failed: %w", err)
		}
		evidence.observe(event)
		// Only keep text from the final response event — intermediate
		// tool-thinking events produce disjointed fragments that get
		// concatenated into garbled output.
		if event.IsFinalResponse() {
			if text := extractContentText(event.Content); text != "" {
				lastFinalText = text
			}
		}
		if evidence.toolResponseCount() >= maxToolIterations {
			a.logWarn(ctx, "whatsappagent: max tool calls reached; returning best-effort reply", "max_tool_calls", maxToolIterations)
			break
		}
	}

	reply := strings.TrimSpace(lastFinalText)

	// Grounding runs in log-only mode: detect issues for observability but
	// never block or replace the LLM reply. The model is capable enough to
	// use tools and interpret results correctly.
	if decision := detectGroundingIssue(reply, evidence); decision.Code != "" {
		a.logWarn(ctx, "whatsappagent: grounding issue observed (log-only)",
			"reason", decision.Code,
			"unsupported_facts", decision.UnsupportedFacts,
			"tool_response_names", evidence.toolNames(),
			"tool_response_count", evidence.toolResponseCount())
	}

	return AgentRunResult{
		Reply:             reply,
		ToolResponseNames: evidence.toolNames(),
		ToolResponseCount: evidence.toolResponseCount(),
	}, nil
}

func (a *Agent) loggerWithContext(ctx context.Context) *logger.Logger {
	if a == nil || a.log == nil {
		return nil
	}
	return a.log.WithContext(ctx)
}

func (a *Agent) logInfo(ctx context.Context, message string, args ...any) {
	if lg := a.loggerWithContext(ctx); lg != nil {
		lg.Info(message, args...)
	}
}

func (a *Agent) logWarn(ctx context.Context, message string, args ...any) {
	if lg := a.loggerWithContext(ctx); lg != nil {
		lg.Warn(message, args...)
	}
}

func extractContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range content.Parts {
		if part.Thought {
			continue
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func newReplyGroundingEvidence() *replyGroundingEvidence {
	return &replyGroundingEvidence{toolResponseNames: make(map[string]int)}
}

func (e *replyGroundingEvidence) observe(event *session.Event) {
	if e == nil || event == nil || event.Content == nil {
		return
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionResponse == nil {
			continue
		}
		response := part.FunctionResponse
		name := strings.TrimSpace(response.Name)
		if name == "" {
			continue
		}
		e.toolResponseNames[name]++
		payload := marshalToolResponsePayload(response.Response)
		e.toolResponses = append(e.toolResponses, toolResponseObservation{Name: name, Payload: payload})
	}
}

func (e *replyGroundingEvidence) hasToolResponse(names ...string) bool {
	if e == nil {
		return false
	}
	for _, name := range names {
		if e.toolResponseNames[strings.TrimSpace(name)] > 0 {
			return true
		}
	}
	return false
}

func (e *replyGroundingEvidence) toolNames() []string {
	if e == nil || len(e.toolResponseNames) == 0 {
		return nil
	}
	names := make([]string, 0, len(e.toolResponseNames))
	for name := range e.toolResponseNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *replyGroundingEvidence) toolResponseCount() int {
	if e == nil {
		return 0
	}
	return len(e.toolResponses)
}

func (e *replyGroundingEvidence) payloadsForTools(names ...string) []string {
	if e == nil || len(e.toolResponses) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	payloads := make([]string, 0, len(e.toolResponses))
	for _, response := range e.toolResponses {
		if _, ok := allowed[response.Name]; ok && strings.TrimSpace(response.Payload) != "" {
			payloads = append(payloads, response.Payload)
		}
	}
	return payloads
}

func marshalToolResponsePayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func detectGroundingIssue(reply string, evidence *replyGroundingEvidence) groundingDecision {
	// Check all grounding categories instead of short-circuiting on the first
	// failure. This ensures that a false positive in one domain does not mask a
	// real issue in another, and that all unsupported facts are reported.
	if d := checkQuoteGrounding(reply, evidence); d.Code != "" {
		return d
	}
	if d := checkAppointmentGrounding(reply, evidence); d.Code != "" {
		return d
	}
	if d := checkLeadGrounding(reply, evidence); d.Code != "" {
		return d
	}
	return groundingDecision{}
}

var (
	quoteTools       = []string{"GetQuotes", "DraftQuote", "GenerateQuote", "SendQuotePDF"}
	quoteDataTools   = []string{"GetQuotes"}
	appointmentTools = []string{"GetAppointments", "GetAvailableVisitSlots", "ScheduleVisit", "RescheduleVisit", "CancelVisit", "GetMyJobs", "GetPartnerJobDetails", "UpdateAppointmentStatus"}
	leadTools        = []string{"SearchLeads", "GetLeadDetails", "GetEnergyLabel", "GetLeadTasks", "CreateLead", "UpdateLeadDetails", "GetNavigationLink", "GetMyJobs", "GetPartnerJobDetails", "GetQuotes"}
)

func checkQuoteGrounding(reply string, evidence *replyGroundingEvidence) groundingDecision {
	quoteFacts := extractQuoteFacts(reply)
	if len(quoteFacts) == 0 {
		return groundingDecision{}
	}
	if !evidence.hasToolResponse(quoteTools...) {
		return groundingDecision{Code: "quote_details_without_quote_tool", UnsupportedFacts: quoteFacts}
	}
	if evidence.hasToolResponse(quoteDataTools...) {
		if unsupported := unsupportedQuoteFacts(reply, evidence.payloadsForTools(quoteDataTools...)); len(unsupported) > 0 {
			return groundingDecision{Code: "quote_fact_not_in_tool_result", UnsupportedFacts: unsupported}
		}
	}
	return groundingDecision{}
}

func checkAppointmentGrounding(reply string, evidence *replyGroundingEvidence) groundingDecision {
	appointmentFacts := extractAppointmentFacts(reply)
	if len(appointmentFacts) == 0 {
		return groundingDecision{}
	}
	// Appointment facts: only flag dates/times as appointment facts when they
	// cannot be explained by quote tool payloads. Dates that already appear in
	// a GetQuotes payload (e.g. created_at) are metadata, not appointment data.
	if !evidence.hasToolResponse(appointmentTools...) {
		unexplained := filterFactsNotInPayloads(appointmentFacts, evidence.payloadsForTools(quoteDataTools...), appointmentFactVariants)
		if len(unexplained) > 0 {
			return groundingDecision{Code: "appointment_details_without_appointment_tool", UnsupportedFacts: unexplained}
		}
		return groundingDecision{}
	}
	if unsupported := unsupportedAppointmentFacts(reply, evidence.payloadsForTools(appointmentTools...)); len(unsupported) > 0 {
		return groundingDecision{Code: "appointment_fact_not_in_tool_result", UnsupportedFacts: unsupported}
	}
	return groundingDecision{}
}

func checkLeadGrounding(reply string, evidence *replyGroundingEvidence) groundingDecision {
	leadFacts := extractLeadFacts(reply)
	if len(leadFacts) == 0 {
		return groundingDecision{}
	}
	if !evidence.hasToolResponse(leadTools...) {
		return groundingDecision{Code: "lead_details_without_lead_tool", UnsupportedFacts: leadFacts}
	}
	if unsupported := unsupportedLeadFacts(reply, evidence.payloadsForTools(leadTools...)); len(unsupported) > 0 {
		return groundingDecision{Code: "lead_fact_not_in_tool_result", UnsupportedFacts: unsupported}
	}
	return groundingDecision{}
}

func unsupportedQuoteFacts(reply string, payloads []string) []string {
	unsupported := make([]string, 0)
	for _, fact := range extractQuoteFacts(reply) {
		if payloadsContainAnyFactVariant(payloads, quoteFactVariants(fact)) {
			continue
		}
		unsupported = append(unsupported, fact)
	}
	return unsupported
}

func unsupportedAppointmentFacts(reply string, payloads []string) []string {
	return unsupportedFactsFromCandidates(extractAppointmentFacts(reply), payloads, appointmentFactVariants)
}

// filterFactsNotInPayloads returns only those facts whose variants do NOT
// appear in any of the given payloads. This is used to exclude date/time
// facts that originate from quote or lead tool responses before treating
// them as appointment facts.
func filterFactsNotInPayloads(facts []string, payloads []string, variants func(string) []string) []string {
	if len(payloads) == 0 {
		return facts
	}
	unexplained := make([]string, 0, len(facts))
	for _, fact := range facts {
		if !payloadsContainAnyFactVariant(payloads, variants(fact)) {
			unexplained = append(unexplained, fact)
		}
	}
	return unexplained
}

func unsupportedLeadFacts(reply string, payloads []string) []string {
	unsupported := make([]string, 0)
	for _, fact := range extractLeadFacts(reply) {
		if isAddressFact(fact) {
			if addressFactSupported(fact, payloads) {
				continue
			}
			unsupported = append(unsupported, fact)
			continue
		}
		if payloadsContainAnyFactVariant(payloads, leadFactVariants(fact)) {
			continue
		}
		unsupported = append(unsupported, fact)
	}
	return unsupported
}

func unsupportedFactsFromCandidates(facts, payloads []string, variants func(string) []string) []string {
	if len(facts) == 0 {
		return nil
	}
	unsupported := make([]string, 0, len(facts))
	for _, fact := range facts {
		if !payloadsContainAnyFactVariant(payloads, variants(fact)) {
			unsupported = append(unsupported, fact)
		}
	}
	return unsupported
}

func payloadsContainAnyFactVariant(payloads, variants []string) bool {
	if len(payloads) == 0 || len(variants) == 0 {
		return false
	}
	for _, payload := range payloads {
		normalizedPayload := normalizeFactText(payload)
		for _, variant := range variants {
			if variant == "" {
				continue
			}
			if strings.Contains(normalizedPayload, normalizeFactText(variant)) {
				return true
			}
		}
	}
	return false
}

func extractQuoteFacts(reply string) []string {
	facts := make([]string, 0, 4)
	facts = append(facts, reCurrencyAmount.FindAllString(reply, -1)...)
	cleanReply := strings.ReplaceAll(reply, "*", "")
	for _, match := range reQuoteLine.FindAllStringSubmatch(cleanReply, -1) {
		if len(match) < 2 {
			continue
		}
		quoteNumber := strings.TrimSpace(match[1])
		if looksLikeQuoteNumber(quoteNumber) {
			facts = append(facts, quoteNumber)
		}
	}
	return uniqueStrings(facts)
}

func extractAppointmentFacts(reply string) []string {
	if !hasAppointmentContext(reply) {
		return nil
	}
	facts := make([]string, 0, 4)
	facts = append(facts, reClockTime.FindAllString(reply, -1)...)
	facts = append(facts, reISODate.FindAllString(reply, -1)...)
	facts = append(facts, reNumericDate.FindAllString(reply, -1)...)
	facts = append(facts, reDutchDate.FindAllString(reply, -1)...)
	return uniqueStrings(facts)
}

func hasAppointmentContext(reply string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(reply, "*", ""))
	for _, keyword := range appointmentContextKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func extractLeadFacts(reply string) []string {
	facts := make([]string, 0, 4)
	facts = append(facts, reEmail.FindAllString(reply, -1)...)
	facts = append(facts, extractPhoneFacts(reply)...)
	cleanReply := strings.ReplaceAll(reply, "*", "")
	facts = append(facts, extractAddressFacts(cleanReply)...)
	facts = append(facts, extractStatusFacts(cleanReply)...)
	return uniqueStrings(facts)
}

func extractPhoneFacts(reply string) []string {
	facts := make([]string, 0, 2)
	for _, loc := range rePhone.FindAllStringIndex(reply, -1) {
		if loc[0] > 0 && reply[loc[0]-1] >= '0' && reply[loc[0]-1] <= '9' {
			continue
		}
		facts = append(facts, strings.TrimSpace(reply[loc[0]:loc[1]]))
	}
	return facts
}

func extractAddressFacts(reply string) []string {
	facts := make([]string, 0, 2)
	for _, match := range reAddressLine.FindAllStringSubmatch(reply, -1) {
		if len(match) < 2 {
			continue
		}
		address := strings.TrimSpace(match[1])
		if address != "" {
			facts = append(facts, address)
		}
	}
	return facts
}

func extractStatusFacts(reply string) []string {
	facts := make([]string, 0, 2)
	for _, match := range reStatusLine.FindAllStringSubmatch(reply, -1) {
		if len(match) < 2 {
			continue
		}
		status := trimStatusFact(match[1])
		if status != "" {
			facts = append(facts, status)
		}
	}
	return facts
}

func trimStatusFact(status string) string {
	trimmed := strings.TrimSpace(status)
	for _, stringToTrim := range []string{" (", ", "} {
		if idx := strings.Index(trimmed, stringToTrim); idx > 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
	}
	for _, sep := range []string{" – ", " — ", " - "} {
		if idx := strings.Index(trimmed, sep); idx > 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
	}
	trimmed = strings.TrimRight(trimmed, ".,;):")
	return strings.TrimSpace(trimmed)
}

func currencyFactVariants(fact string) []string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(fact), "€"))
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil
	}
	digits := strings.NewReplacer(".", "", ",", "", " ", "").Replace(trimmed)
	whole := strings.NewReplacer(".", "", " ", "").Replace(trimmed)
	whole = strings.ReplaceAll(whole, ",00", "")
	variants := []string{trimmed, digits, whole, strings.ReplaceAll(trimmed, ",", ".")}
	// When no cents suffix is present (e.g. "€1.500" without ",00"), also generate
	// the cent representation (digits + "00") so it matches total_cents in payloads.
	if !strings.Contains(trimmed, ",") {
		variants = append(variants, digits+"00")
	}
	return uniqueStrings(variants)
}

func quoteFactVariants(fact string) []string {
	if reCurrencyAmount.MatchString(fact) {
		return currencyFactVariants(fact)
	}
	trimmed := strings.TrimSpace(fact)
	variants := []string{trimmed, strings.ToUpper(trimmed), strings.ToLower(trimmed)}
	return uniqueStrings(variants)
}

func appointmentFactVariants(fact string) []string {
	variants := []string{strings.TrimSpace(fact)}
	if parsed := parseDateFact(fact); parsed != nil {
		variants = append(variants,
			fmt.Sprintf("%04d-%02d-%02d", parsed.year, parsed.month, parsed.day),
			fmt.Sprintf("%02d-%02d-%04d", parsed.day, parsed.month, parsed.year),
			fmt.Sprintf("%02d/%02d/%04d", parsed.day, parsed.month, parsed.year),
		)
	}
	return uniqueStrings(variants)
}

func leadFactVariants(fact string) []string {
	trimmed := strings.TrimSpace(fact)
	variants := []string{trimmed}
	if rePhone.MatchString(trimmed) {
		digitsOnly := strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, trimmed)
		variants = append(variants, digitsOnly)
	}
	// Add Dutch↔English status translations so the grounding checker accepts
	// the LLM translating tool response statuses (e.g. "Draft" → "Concept").
	if translations, ok := statusTranslations[strings.ToLower(trimmed)]; ok {
		variants = append(variants, translations...)
	}
	return uniqueStrings(variants)
}

// statusTranslations maps Dutch status labels used in LLM replies to their
// English counterparts in tool response payloads, and vice versa.
var statusTranslations = map[string][]string{
	"concept":            {"Draft"},
	"draft":              {"Concept"},
	"verstuurd":          {"Sent"},
	"verzonden":          {"Sent"},
	"sent":               {"Verstuurd", "Verzonden"},
	"openstaand":         {"Sent", "Pending"},
	"pending":            {"Openstaand"},
	"geaccepteerd":       {"Accepted"},
	"goedgekeurd":        {"Accepted", "Approved"},
	"accepted":           {"Geaccepteerd", "Akkoord", "Goedgekeurd"},
	"approved":           {"Geaccepteerd", "Goedgekeurd"},
	"akkoord":            {"Accepted"},
	"afgewezen":          {"Rejected"},
	"rejected":           {"Afgewezen"},
	"verlopen":           {"Expired"},
	"expired":            {"Verlopen"},
	"nieuw":              {"New"},
	"new":                {"Nieuw"},
	"in behandeling":     {"In_Progress"},
	"in_progress":        {"In behandeling"},
	"gepland":            {"Scheduled"},
	"ingepland":          {"Scheduled"},
	"scheduled":          {"Gepland", "Ingepland"},
	"geannuleerd":        {"Cancelled", "Canceled"},
	"cancelled":          {"Geannuleerd"},
	"canceled":           {"Geannuleerd"},
	"contact geprobeerd": {"Attempted_Contact"},
	"attempted_contact":  {"Contact geprobeerd"},
	"herplannen":         {"Needs_Rescheduling"},
	"needs_rescheduling": {"Herplannen"},
}

func looksLikeQuoteNumber(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && reQuoteNumber.MatchString(trimmed)
}

func isAddressFact(fact string) bool {
	trimmed := strings.TrimSpace(fact)
	return strings.Contains(trimmed, ",") || (strings.ContainsAny(trimmed, "0123456789") && strings.Contains(trimmed, " "))
}

func addressFactSupported(fact string, payloads []string) bool {
	tokens := addressFactTokens(fact)
	if len(tokens) == 0 || len(payloads) == 0 {
		return false
	}
	for _, payload := range payloads {
		normalizedPayload := normalizeFactText(payload)
		allPresent := true
		for _, token := range tokens {
			if !strings.Contains(normalizedPayload, normalizeFactText(token)) {
				allPresent = false
				break
			}
		}
		if allPresent {
			return true
		}
	}
	return false
}

func addressFactTokens(fact string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ")
	parts := strings.Fields(replacer.Replace(strings.TrimSpace(fact)))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized := normalizeFactText(trimmed)
		if len(normalized) <= 1 {
			continue
		}
		tokens = append(tokens, trimmed)
	}
	return uniqueStrings(tokens)
}

type parsedDateFact struct {
	day   int
	month int
	year  int
}

func parseDateFact(fact string) *parsedDateFact {
	trimmed := strings.ToLower(strings.TrimSpace(fact))
	if trimmed == "" {
		return nil
	}
	for _, parser := range []func(string) *parsedDateFact{parseISODateFact, parseSlashedDateFact, parseDashedDayFirstDateFact, parseDutchMonthDateFact} {
		if parsed := parser(trimmed); parsed != nil {
			return parsed
		}
	}
	return nil
}

func parseISODateFact(trimmed string) *parsedDateFact {
	var year, month, day int
	if _, err := fmt.Sscanf(trimmed, "%d-%d-%d", &year, &month, &day); err != nil || year <= 1900 {
		return nil
	}
	return &parsedDateFact{day: day, month: month, year: year}
}

func parseSlashedDateFact(trimmed string) *parsedDateFact {
	var day, month, year int
	if _, err := fmt.Sscanf(trimmed, "%d/%d/%d", &day, &month, &year); err != nil || year <= 0 {
		return nil
	}
	return &parsedDateFact{day: day, month: month, year: normalizeTwoDigitYear(year)}
}

func parseDashedDayFirstDateFact(trimmed string) *parsedDateFact {
	var day, month, year int
	if _, err := fmt.Sscanf(trimmed, "%d-%d-%d", &day, &month, &year); err != nil || year <= 0 {
		return nil
	}
	return &parsedDateFact{day: day, month: month, year: normalizeTwoDigitYear(year)}
}

func parseDutchMonthDateFact(trimmed string) *parsedDateFact {
	parts := strings.Fields(trimmed)
	if len(parts) != 3 {
		return nil
	}
	monthNumber, ok := dutchMonthNumbers[parts[1]]
	if !ok {
		return nil
	}
	day, ok := scanInt(parts[0])
	if !ok {
		return nil
	}
	month, ok := scanInt(monthNumber)
	if !ok {
		return nil
	}
	year, ok := scanInt(parts[2])
	if !ok {
		return nil
	}
	return &parsedDateFact{day: day, month: month, year: year}
}

func scanInt(input string) (int, bool) {
	var value int
	if _, err := fmt.Sscanf(input, "%d", &value); err != nil {
		return 0, false
	}
	return value, true
}

func normalizeTwoDigitYear(year int) int {
	if year < 100 {
		return year + 2000
	}
	return year
}

func normalizeFactText(input string) string {
	lower := strings.ToLower(input)
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	unique := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := normalizeFactText(trimmed)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, trimmed)
	}
	return unique
}
