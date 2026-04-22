package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/orchestration"
	apptools "portal_final_backend/internal/tools"
	"portal_final_backend/platform/ai/openaicompat"
)

type Auditor struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	eventBus       events.Bus
	toolDeps       *AuditorToolDeps
}

type AuditorToolDeps struct {
	Repo     repository.LeadsRepository
	EventBus events.Bus

	mu        sync.RWMutex
	tenantID  *uuid.UUID
	leadID    *uuid.UUID
	serviceID *uuid.UUID
}

func (d *AuditorToolDeps) SetContext(tenantID, leadID, serviceID uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenantID = &tenantID
	d.leadID = &leadID
	d.serviceID = &serviceID
}

func (d *AuditorToolDeps) GetContext() (tenantID, leadID, serviceID uuid.UUID, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tenantID == nil || d.leadID == nil || d.serviceID == nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, false
	}
	return *d.tenantID, *d.leadID, *d.serviceID, true
}

func (d *AuditorToolDeps) NewRequestDeps() *AuditorToolDeps {
	return &AuditorToolDeps{Repo: d.Repo, EventBus: d.EventBus}
}

type SubmitAuditResultInput struct {
	Passed  bool     `json:"passed"`
	Summary string   `json:"summary"`
	Missing []string `json:"missing,omitempty"`
}

type SubmitAuditResultOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (d *AuditorToolDeps) handleSubmitAuditResult(ctx tool.Context, input SubmitAuditResultInput) (SubmitAuditResultOutput, error) {
	tenantID, leadID, serviceID, ok := d.GetContext()
	if !ok {
		return SubmitAuditResultOutput{Status: "error", Message: "missing context"}, nil
	}

	missing := make([]string, 0)
	for _, m := range input.Missing {
		m = strings.TrimSpace(m)
		if m != "" {
			missing = append(missing, m)
		}
	}

	summaryText := strings.TrimSpace(input.Summary)
	if summaryText == "" {
		summaryText = "Audit completed."
	}

	// If audit failed or missing info is present, move to Manual_Intervention.
	if !input.Passed || len(missing) > 0 {
		if _, err := d.Repo.UpdatePipelineStage(ctx, serviceID, tenantID, domain.PipelineStageManualIntervention); err != nil {
			log.Printf("auditor: failed to set Manual_Intervention: %v", err)
		}
	}

	// Record transparent timeline event.
	title := repository.EventTitleManualIntervention
	eventType := repository.EventTypeAlert
	actorType := repository.ActorTypeAI
	actorName := "Audit Agent"
	sum := summaryText
	meta := repository.AlertMetadata{
		Trigger: "audit_agent",
	}.ToMap()
	if len(missing) > 0 {
		meta["missing"] = missing
	}
	_, _ = d.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      eventType,
		Title:          title,
		Summary:        &sum,
		Metadata:       meta,
	})

	if d.EventBus != nil {
		d.EventBus.Publish(ctx, events.AuditCompleted{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			Passed:        input.Passed && len(missing) == 0,
			Findings:      missing,
		})
	}

	return SubmitAuditResultOutput{Status: "ok", Message: "audit stored"}, nil
}

func NewAuditor(modelCfg openaicompat.Config, repo repository.LeadsRepository, eventBus events.Bus) (*Auditor, error) {
	kimi := openaicompat.NewModel(modelCfg)
	workspace, err := orchestration.LoadAgentWorkspace("auditor")
	if err != nil {
		return nil, fmt.Errorf("failed to load auditor workspace context: %w", err)
	}

	deps := &AuditorToolDeps{Repo: repo, EventBus: eventBus}

	submitTool, err := apptools.NewSubmitAuditResultTool(func(ctx tool.Context, input SubmitAuditResultInput) (SubmitAuditResultOutput, error) {
		deps, err := GetAuditorDeps(ctx)
		if err != nil {
			return SubmitAuditResultOutput{}, err
		}
		return deps.handleSubmitAuditResult(ctx, input)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SubmitAuditResult tool: %w", err)
	}
	toolsets := orchestration.BuildWorkspaceToolsets(workspace, "auditor_tools", []tool.Tool{submitTool})

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "AuditAgent",
		Model:       kimi,
		Description: "Internal reviewer that validates VisitReports and CallLogs against intake guidelines.",
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create audit agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "auditor",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create auditor runner: %w", err)
	}

	return &Auditor{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "auditor",
		repo:           repo,
		eventBus:       eventBus,
		toolDeps:       deps,
	}, nil
}

func (a *Auditor) AuditVisitReport(ctx context.Context, leadID, serviceID, tenantID, appointmentID uuid.UUID) error {
	reqDeps := a.toolDeps.NewRequestDeps()
	reqDeps.SetContext(tenantID, leadID, serviceID)
	ctx = WithAuditorDeps(ctx, reqDeps)

	service, err := a.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	visitReport, err := a.repo.GetAppointmentVisitReport(ctx, appointmentID, tenantID)
	if err != nil {
		if err == repository.ErrNotFound {
			log.Printf("auditor: visit report not found for appointment=%s", appointmentID)
			return nil
		}
		return err
	}

	notes, _ := a.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	intakeContext := buildIntakeContext(ctx, a.repo, tenantID, service.ServiceType)

	prompt := buildVisitReportAuditPrompt(service.ServiceType, intakeContext, visitReport, notes)
	return a.runWithPrompt(ctx, prompt, leadID)
}

func (a *Auditor) AuditCallLog(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	reqDeps := a.toolDeps.NewRequestDeps()
	reqDeps.SetContext(tenantID, leadID, serviceID)
	ctx = WithAuditorDeps(ctx, reqDeps)

	service, err := a.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	notes, _ := a.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	intakeContext := buildIntakeContext(ctx, a.repo, tenantID, service.ServiceType)
	prompt := buildCallLogAuditPrompt(service.ServiceType, intakeContext, notes)
	return a.runWithPrompt(ctx, prompt, leadID)
}

func (a *Auditor) runWithPrompt(ctx context.Context, promptText string, leadID uuid.UUID) error {
	sessionID := uuid.New().String()
	userID := "auditor-" + leadID.String()
	return runPromptSession(ctx, promptRunRequest{
		SessionService:       a.sessionService,
		Runner:               a.runner,
		AppName:              a.appName,
		UserID:               userID,
		SessionID:            sessionID,
		UserMessage:          &genai.Content{Role: "user", Parts: []*genai.Part{{Text: promptText}}},
		CreateSessionMessage: "failed to create auditor session",
		RunFailureMessage:    "auditor run failed",
		TraceLabel:           "auditor",
	},
		func(event *session.Event) {
			_ = event
		},
	)
}

func buildIntakeContext(ctx context.Context, repo repository.LeadsRepository, tenantID uuid.UUID, currentServiceType string) string {
	services, err := repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return "No intake requirements available."
	}
	key := strings.ToLower(strings.TrimSpace(currentServiceType))
	for i := range services {
		svc := services[i]
		nameKey := strings.ToLower(strings.TrimSpace(svc.Name))
		if nameKey == key || strings.ToLower(strings.TrimSpace(getSlugLike(svc.Name))) == key {
			var sb strings.Builder
			_, _ = fmt.Fprintf(&sb, "Selected service type: %s\n\n", currentServiceType)
			if svc.Description != nil && strings.TrimSpace(*svc.Description) != "" {
				sb.WriteString("Description: " + strings.TrimSpace(*svc.Description) + "\n")
			}
			if svc.IntakeGuidelines != nil && strings.TrimSpace(*svc.IntakeGuidelines) != "" {
				sb.WriteString("Intake Requirements:\n")
				sb.WriteString(strings.TrimSpace(*svc.IntakeGuidelines) + "\n")
			} else {
				sb.WriteString("Intake Requirements: Not specified.\n")
			}
			return sb.String()
		}
	}
	return "Intake Requirements for selected service type: Not found."
}

func buildVisitReportAuditPrompt(serviceType string, intakeContext string, report *repository.AppointmentVisitReport, notes []repository.LeadNote) string {
	var sb strings.Builder
	sb.WriteString("You are the Audit Agent (internal reviewer).\n")
	sb.WriteString("You may reason step-by-step internally, but your final output must contain only the required tool calls.\n")
	sb.WriteString("Compare the submitted visit report against the intake requirements.\n")
	sb.WriteString("If anything required is missing or too thin (e.g., 'looks fine' without details), list what is missing.\n")
	sb.WriteString("Then call SubmitAuditResult.\n\n")

	sb.WriteString("SERVICE TYPE:\n" + wrapReferenceBlock(serviceType) + "\n\n")
	sb.WriteString("INTAKE GUIDELINES:\n" + wrapReferenceBlock(intakeContext) + "\n\n")

	sb.WriteString("VISIT REPORT:\n")
	sb.WriteString(wrapReferenceBlock(strings.Join([]string{
		"- Measurements: " + getValue(report.Measurements),
		"- Access difficulty: " + getValue(report.AccessDifficulty),
		"- Notes: " + getValue(report.Notes),
	}, "\n")) + "\n\n")

	// Include a small note context window.
	if len(notes) > 0 {
		max := 5
		if len(notes) < max {
			max = len(notes)
		}
		recentNotes := make([]string, 0, max)
		for i := 0; i < max; i++ {
			n := notes[i]
			recentNotes = append(recentNotes, fmt.Sprintf("- [%s] %s: %s", n.Type, n.AuthorEmail, sanitizeUserInput(n.Body, 300)))
		}
		sb.WriteString("RECENT NOTES (context):\n")
		sb.WriteString(wrapReferenceBlock(strings.Join(recentNotes, "\n")) + "\n")
		sb.WriteString("\n")
	}

	sb.WriteString("OUTPUT RULES:\n")
	sb.WriteString("- Final output must contain only the SubmitAuditResult tool call.\n")
	sb.WriteString("- If missing required info: SubmitAuditResult(passed=false, missing=[...], summary=short Dutch explanation).\n")
	sb.WriteString("- If sufficient: SubmitAuditResult(passed=true, missing=[], summary=short confirmation).\n")
	return sb.String()
}

func buildCallLogAuditPrompt(serviceType string, intakeContext string, notes []repository.LeadNote) string {
	var sb strings.Builder
	sb.WriteString("You are the Audit Agent (internal reviewer).\n")
	sb.WriteString("You may reason step-by-step internally, but your final output must contain only the required tool calls.\n")
	sb.WriteString("Audit the latest call log / notes against the intake requirements.\n")
	sb.WriteString("Then call SubmitAuditResult.\n\n")

	sb.WriteString("SERVICE TYPE:\n" + wrapReferenceBlock(serviceType) + "\n\n")
	sb.WriteString("INTAKE GUIDELINES:\n" + wrapReferenceBlock(intakeContext) + "\n\n")

	max := 10
	if len(notes) < max {
		max = len(notes)
	}
	sb.WriteString("RECENT NOTES:\n")
	recentNotes := make([]string, 0, max)
	for i := 0; i < max; i++ {
		n := notes[i]
		recentNotes = append(recentNotes, fmt.Sprintf("- [%s] %s: %s", n.Type, n.AuthorEmail, sanitizeUserInput(n.Body, 400)))
	}
	sb.WriteString(wrapReferenceBlock(strings.Join(recentNotes, "\n")) + "\n\n")

	sb.WriteString("OUTPUT RULES:\n")
	sb.WriteString("- Final output must contain only the SubmitAuditResult tool call.\n")
	sb.WriteString("- If missing required info: SubmitAuditResult(passed=false, missing=[...], summary=short Dutch explanation).\n")
	sb.WriteString("- If sufficient: SubmitAuditResult(passed=true, missing=[], summary=short confirmation).\n")
	return sb.String()
}
