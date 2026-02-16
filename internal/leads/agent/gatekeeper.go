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
	"portal_final_backend/internal/leads/repository"
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
func NewGatekeeper(apiKey string, repo repository.LeadsRepository, eventBus events.Bus) (*Gatekeeper, error) {
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &ToolDependencies{
		Repo:     repo,
		EventBus: eventBus,
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
	g.toolDeps.SetActor("AI", "Gatekeeper")
	g.toolDeps.ResetToolCallTracking() // Reset before each run

	lead, err := g.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := g.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

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

	intakeContext := g.buildServiceContext(ctx, tenantID)
	promptText := buildGatekeeperPrompt(lead, service, notes, intakeContext, attachments, photoAnalysis)

	log.Printf("gatekeeper: starting runWithPrompt for lead=%s service=%s", leadID, serviceID)
	if err := g.runWithPrompt(ctx, promptText, leadID); err != nil {
		log.Printf("gatekeeper: runWithPrompt failed for lead=%s: %v", leadID, err)
		return err
	}
	log.Printf("gatekeeper: runWithPrompt completed for lead=%s", leadID)

	// Validate that SaveAnalysis was called - if not, create fallback
	wasCalled := g.toolDeps.WasSaveAnalysisCalled()
	log.Printf("gatekeeper: WasSaveAnalysisCalled()=%v for lead=%s service=%s", wasCalled, leadID, serviceID)
	if !wasCalled {
		log.Printf("gatekeeper: SaveAnalysis was NOT called by agent for lead=%s service=%s, creating fallback", leadID, serviceID)
		g.createFallbackAnalysis(ctx, lead, leadID, serviceID, tenantID)
	} else {
		log.Printf("gatekeeper: SaveAnalysis was called successfully for lead=%s service=%s", leadID, serviceID)
	}

	return nil
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
	analysisMetadata := map[string]any{
		"urgencyLevel":            "Medium",
		"recommendedAction":       "RequestInfo",
		"leadQuality":             "Potential",
		"preferredContactChannel": channel,
		"suggestedContactMessage": fmt.Sprintf("Beste %s, bedankt voor uw aanvraag. Kunt u ons meer details geven over uw project?", lead.ConsumerFirstName),
		"missingInformation":      []string{"Intake validatie niet voltooid door AI"},
		"fallback":                true,
	}
	_, _ = g.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      "AI",
		ActorName:      "Gatekeeper",
		EventType:      "ai",
		Title:          "Gatekeeper-triage (fallback)",
		Summary:        &summary,
		Metadata:       analysisMetadata,
	})

	// Store for stage_change event if needed
	g.toolDeps.SetLastAnalysisMetadata(analysisMetadata)
	log.Printf("gatekeeper: created fallback analysis for lead=%s service=%s", leadID, serviceID)
}

func (g *Gatekeeper) buildServiceContext(ctx context.Context, tenantID uuid.UUID) string {
	services, err := g.repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return "No intake requirements available."
	}

	var sb strings.Builder
	for _, svc := range services {
		sb.WriteString(fmt.Sprintf("### %s\n", svc.Name))
		if svc.Description != nil && *svc.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", *svc.Description))
		}
		if svc.IntakeGuidelines != nil && *svc.IntakeGuidelines != "" {
			sb.WriteString(fmt.Sprintf("Intake Requirements: %s\n", *svc.IntakeGuidelines))
		} else {
			sb.WriteString("Intake Requirements: Not specified.\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
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
