package agent

import (
	"context"
	"errors"
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
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/platform/ai/moonshot"
)

// Gatekeeper validates intake requirements and advances pipeline stage.
type Gatekeeper struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	runMu          sync.Mutex
}

// NewGatekeeper creates a Gatekeeper agent.
func NewGatekeeper(apiKey string, repo repository.LeadsRepository, eventBus events.Bus, scorer *scoring.Service) (*Gatekeeper, error) {
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &ToolDependencies{
		Repo:     repo,
		EventBus: eventBus,
		Scorer:   scorer,
	}

	updateStageTool, err := createUpdatePipelineStageTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdatePipelineStage tool: %w", err)
	}
	saveAnalysisTool, err := createSaveAnalysisTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build SaveAnalysis tool: %w", err)
	}
	updateServiceTypeTool, err := createUpdateLeadServiceTypeTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdateLeadServiceType tool: %w", err)
	}
	updateLeadDetailsTool, err := createUpdateLeadDetailsTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdateLeadDetails tool: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "Gatekeeper",
		Model:       kimi,
		Description: "Validates intake requirements and advances the lead pipeline.",
		Instruction: "You validate intake requirements and advance the pipeline stage.",
		Tools:       []tool.Tool{saveAnalysisTool, updateLeadDetailsTool, updateServiceTypeTool, updateStageTool},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gatekeeper agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "gatekeeper",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gatekeeper runner: %w", err)
	}

	return &Gatekeeper{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "gatekeeper",
		repo:           repo,
		toolDeps:       deps,
	}, nil
}

// Run executes the gatekeeper for a lead service.
func (g *Gatekeeper) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	g.runMu.Lock()
	defer g.runMu.Unlock()

	g.toolDeps.SetTenantID(tenantID)
	g.toolDeps.SetLeadContext(leadID, serviceID)
	g.toolDeps.SetActor(repository.ActorTypeAI, repository.ActorNameGatekeeper)
	g.toolDeps.ResetToolCallTracking() // Reset before each run

	lead, service, err := g.fetchLeadAndService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		return err
	}

	notes, attachments, photoAnalysis := g.fetchServiceContext(ctx, leadID, serviceID, tenantID)
	intakeContext := g.buildServiceContext(ctx, tenantID, service.ServiceType)
	if err := g.runGatekeeperPrompt(ctx, gatekeeperPromptRequest{
		leadID:        leadID,
		serviceID:     serviceID,
		lead:          lead,
		service:       service,
		notes:         notes,
		intakeContext: intakeContext,
		attachments:   attachments,
		photoAnalysis: photoAnalysis,
	}); err != nil {
		return err
	}

	// Validate that SaveAnalysis was called - if not, create fallback
	wasCalled := g.toolDeps.WasSaveAnalysisCalled()
	log.Printf("gatekeeper: WasSaveAnalysisCalled()=%v for lead=%s service=%s", wasCalled, leadID, serviceID)
	if !wasCalled {
		log.Printf("gatekeeper: SaveAnalysis was NOT called by agent for lead=%s service=%s, creating fallback", leadID, serviceID)
		g.createFallbackAnalysis(ctx, lead, leadID, serviceID, tenantID)
	} else {
		log.Printf("gatekeeper: SaveAnalysis was called successfully for lead=%s service=%s", leadID, serviceID)
	}

	g.maybeAutoDisqualifyJunk(ctx, leadID, serviceID, tenantID, service)

	return nil
}

func (g *Gatekeeper) fetchLeadAndService(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) (repository.Lead, repository.LeadService, error) {
	lead, err := g.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return repository.Lead{}, repository.LeadService{}, err
	}
	service, err := g.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return repository.Lead{}, repository.LeadService{}, err
	}
	return lead, service, nil
}

func (g *Gatekeeper) fetchServiceContext(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) ([]repository.LeadNote, []repository.Attachment, *repository.PhotoAnalysis) {
	notes, err := g.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		log.Printf("gatekeeper notes fetch failed: %v", err)
		notes = nil
	}

	attachments, err := g.repo.ListAttachmentsByService(ctx, serviceID, tenantID)
	if err != nil {
		log.Printf("gatekeeper attachments fetch failed: %v", err)
		attachments = nil
	}

	var photoAnalysis *repository.PhotoAnalysis
	if pa, err := g.repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID); err == nil {
		photoAnalysis = &pa
	} else if !errors.Is(err, repository.ErrPhotoAnalysisNotFound) {
		log.Printf("gatekeeper photo analysis fetch failed: %v", err)
	}

	return notes, attachments, photoAnalysis
}

type gatekeeperPromptRequest struct {
	leadID        uuid.UUID
	serviceID     uuid.UUID
	lead          repository.Lead
	service       repository.LeadService
	notes         []repository.LeadNote
	intakeContext string
	attachments   []repository.Attachment
	photoAnalysis *repository.PhotoAnalysis
}

func (g *Gatekeeper) runGatekeeperPrompt(ctx context.Context, req gatekeeperPromptRequest) error {
	promptText := buildGatekeeperPrompt(req.lead, req.service, req.notes, req.intakeContext, req.attachments, req.photoAnalysis)

	log.Printf("gatekeeper: starting runWithPrompt for lead=%s service=%s", req.leadID, req.serviceID)
	if err := g.runWithPrompt(ctx, promptText, req.leadID); err != nil {
		log.Printf("gatekeeper: runWithPrompt failed for lead=%s: %v", req.leadID, err)
		return err
	}
	log.Printf("gatekeeper: runWithPrompt completed for lead=%s", req.leadID)
	return nil
}

func (g *Gatekeeper) maybeAutoDisqualifyJunk(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, service repository.LeadService) {
	// Autonomous "Junk" disposal: if the latest analysis marks this lead as Junk,
	// automatically move it to Disqualified/Lost and record a transparent timeline event.
	// This lives here (instead of only the orchestrator) so it applies regardless of who triggered Gatekeeper.Run.
	if service.Status == domain.LeadStatusDisqualified {
		return
	}

	analysis, err := g.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		if err != repository.ErrNotFound {
			log.Printf("gatekeeper: failed to fetch latest AI analysis for junk check: %v", err)
		}
		return
	}
	if analysis.LeadQuality != "Junk" {
		return
	}

	log.Printf("gatekeeper: auto-disqualifying Junk lead (service=%s lead=%s)", serviceID, leadID)

	if _, err := g.repo.UpdatePipelineStage(ctx, serviceID, tenantID, domain.PipelineStageLost); err != nil {
		log.Printf("gatekeeper: failed to set pipeline stage to Lost during junk auto-disqualify: %v", err)
	}
	if _, err := g.repo.UpdateServiceStatus(ctx, serviceID, tenantID, domain.LeadStatusDisqualified); err != nil {
		log.Printf("gatekeeper: failed to set service status to Disqualified during junk auto-disqualify: %v", err)
		return
	}

	summary := "AI detected Junk quality. Lead automatically moved to Disqualified."
	_, _ = g.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeAI,
		ActorName:      repository.ActorNameGatekeeper,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleAutoDisqualified,
		Summary:        &summary,
		Metadata: repository.AutoDisqualifyMetadata{
			LeadQuality:       analysis.LeadQuality,
			RecommendedAction: analysis.RecommendedAction,
			AnalysisID:        analysis.ID,
			Reason:            "junk_quality",
		}.ToMap(),
	})

	if g.toolDeps != nil && g.toolDeps.EventBus != nil {
		g.toolDeps.EventBus.Publish(ctx, events.LeadAutoDisqualified{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			Reason:        "junk_quality",
		})
	}
}

// createFallbackAnalysis creates a minimal analysis when the agent fails to call SaveAnalysis
func (g *Gatekeeper) createFallbackAnalysis(ctx context.Context, lead repository.Lead, leadID, serviceID, tenantID uuid.UUID) {
	// Determine preferred channel based on available contact info
	channel := "Email"
	if strings.TrimSpace(lead.ConsumerPhone) != "" {
		channel = "WhatsApp"
	}

	// Create a default analysis record
	_, err := g.repo.CreateAIAnalysis(ctx, repository.CreateAIAnalysisParams{
		LeadID:                  leadID,
		OrganizationID:          tenantID,
		LeadServiceID:           serviceID,
		UrgencyLevel:            "Medium",
		UrgencyReason:           nil,
		LeadQuality:             "Potential",
		RecommendedAction:       "RequestInfo",
		MissingInformation:      []string{"Intake validatie niet voltooid door AI"},
		PreferredContactChannel: channel,
		SuggestedContactMessage: fmt.Sprintf("Beste %s, bedankt voor uw aanvraag. Kunt u ons meer details geven over uw project?", lead.ConsumerFirstName),
		Summary:                 "AI analyse kon niet worden voltooid. Handmatige beoordeling vereist.",
	})
	if err != nil {
		log.Printf("gatekeeper: failed to create fallback analysis: %v", err)
		return
	}

	// Create timeline event for the fallback
	summary := "AI analyse kon niet worden voltooid. Handmatige beoordeling vereist."
	fallbackMeta := repository.AIAnalysisMetadata{
		UrgencyLevel:            "Medium",
		RecommendedAction:       "RequestInfo",
		LeadQuality:             "Potential",
		PreferredContactChannel: channel,
		SuggestedContactMessage: fmt.Sprintf("Beste %s, bedankt voor uw aanvraag. Kunt u ons meer details geven over uw project?", lead.ConsumerFirstName),
		MissingInformation:      []string{"Intake validatie niet voltooid door AI"},
		Fallback:                true,
	}
	analysisMetadata := fallbackMeta.ToMap()
	_, _ = g.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeAI,
		ActorName:      repository.ActorNameGatekeeper,
		EventType:      repository.EventTypeAI,
		Title:          repository.EventTitleGatekeeperFallback,
		Summary:        &summary,
		Metadata:       analysisMetadata,
	})

	// Store for stage_change event if needed
	g.toolDeps.SetLastAnalysisMetadata(analysisMetadata)
	log.Printf("gatekeeper: created fallback analysis for lead=%s service=%s", leadID, serviceID)
}

func (g *Gatekeeper) buildServiceContext(ctx context.Context, tenantID uuid.UUID, currentServiceType string) string {
	services, err := g.repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return "No intake requirements available."
	}

	currentKey := strings.ToLower(strings.TrimSpace(currentServiceType))

	// Keep the context focused: list all active types (for awareness), but include
	// the detailed intake guidelines only for the currently selected service type.
	activeNames := make([]string, 0, len(services))
	var selected *repository.ServiceContextDefinition
	for i := range services {
		svc := services[i]
		activeNames = append(activeNames, svc.Name)
		if strings.ToLower(strings.TrimSpace(svc.Name)) == currentKey || strings.ToLower(strings.TrimSpace(getSlugLike(svc.Name))) == currentKey {
			selected = &services[i]
		}
	}

	var sb strings.Builder
	sb.WriteString("Active service types: " + strings.Join(activeNames, ", ") + "\n\n")
	sb.WriteString(fmt.Sprintf("Selected service type (current): %s\n\n", currentServiceType))

	if selected == nil {
		sb.WriteString("Intake Requirements for selected service type: Not found (service type may be inactive or renamed).\n")
		sb.WriteString("If intake requirements are missing, move to Nurturing and request the missing details.\n")
		return sb.String()
	}

	if selected.Description != nil && strings.TrimSpace(*selected.Description) != "" {
		sb.WriteString("Description: " + strings.TrimSpace(*selected.Description) + "\n")
	}
	if selected.IntakeGuidelines != nil && strings.TrimSpace(*selected.IntakeGuidelines) != "" {
		sb.WriteString("Intake Requirements (includes any heuristics/checklist text configured by tenant):\n")
		sb.WriteString(strings.TrimSpace(*selected.IntakeGuidelines) + "\n")
	} else {
		sb.WriteString("Intake Requirements: Not specified.\n")
	}

	return sb.String()
}

// getSlugLike is a minimal helper to reduce mismatches when a tenant uses a slug-like
// service type string (e.g. "insulation") while the DB uses a display name (e.g. "Insulation").
func getSlugLike(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func (g *Gatekeeper) runWithPrompt(ctx context.Context, promptText string, leadID uuid.UUID) error {
	sessionID := uuid.New().String()
	userID := "gatekeeper-" + leadID.String()

	_, err := g.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   g.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create gatekeeper session: %w", err)
	}
	defer func() {
		_ = g.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   g.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	for event := range g.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		_ = event
	}

	return nil
}
