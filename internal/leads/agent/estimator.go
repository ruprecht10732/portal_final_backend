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
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/ai/moonshot"
	"portal_final_backend/platform/qdrant"
)

// Estimator determines scope and pricing estimates.
type Estimator struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	runMu          sync.Mutex
}

// EstimatorConfig holds configuration for creating an Estimator agent.
type EstimatorConfig struct {
	APIKey               string
	Repo                 repository.LeadsRepository
	EventBus             events.Bus
	EmbeddingClient      *embeddings.Client  // Optional: enables product search
	QdrantClient         *qdrant.Client      // Optional: fallback collection search
	BouwmaatQdrantClient *qdrant.Client      // Optional: secondary fallback collection search
	CatalogQdrantClient  *qdrant.Client      // Optional: catalog collection search
	CatalogReader        ports.CatalogReader // Optional: hydrate search results from DB
	QuoteDrafter         ports.QuoteDrafter  // Optional: draft quotes from agent
}

// NewEstimator creates an Estimator agent.
func NewEstimator(cfg EstimatorConfig) (*Estimator, error) {
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          cfg.APIKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &ToolDependencies{
		Repo:                 cfg.Repo,
		EventBus:             cfg.EventBus,
		EmbeddingClient:      cfg.EmbeddingClient,
		QdrantClient:         cfg.QdrantClient,
		BouwmaatQdrantClient: cfg.BouwmaatQdrantClient,
		CatalogQdrantClient:  cfg.CatalogQdrantClient,
		CatalogReader:        cfg.CatalogReader,
		QuoteDrafter:         cfg.QuoteDrafter,
	}

	saveEstimationTool, err := createSaveEstimationTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build SaveEstimation tool: %w", err)
	}

	updateStageTool, err := createUpdatePipelineStageTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdatePipelineStage tool: %w", err)
	}

	calculateEstimateTool, err := createCalculateEstimateTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build CalculateEstimate tool: %w", err)
	}

	calculatorTool, err := createCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build Calculator tool: %w", err)
	}

	// Build the tools list.
	// DraftQuote is always registered because QuoteDrafter is injected after
	// construction via SetQuoteDrafter (to break circular dependencies).
	// handleDraftQuote gracefully returns an error when the drafter is still nil.
	draftQuoteTool, err := createDraftQuoteTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build DraftQuote tool: %w", err)
	}

	tools := []tool.Tool{calculatorTool, calculateEstimateTool, saveEstimationTool, updateStageTool, draftQuoteTool}

	// Optional tool: catalog gap signals for catalog improvement (uses org settings by default).
	listCatalogGapsTool, err := createListCatalogGapsTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build ListCatalogGaps tool: %w", err)
	}
	tools = append(tools, listCatalogGapsTool)

	// Add product search tool if configured
	if deps.IsProductSearchEnabled() {
		searchProductsTool, err := createSearchProductMaterialsTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build SearchProductMaterials tool: %w", err)
		}
		tools = append(tools, searchProductsTool)
		log.Printf("Estimator: product search enabled")
	} else {
		log.Printf("Estimator: product search disabled (embedding or qdrant client not configured)")
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "Estimator",
		Model:       kimi,
		Description: "Technical estimator that scopes work and suggests price ranges.",
		Instruction: "You are a Technical Estimator.",
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create estimator agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "estimator",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create estimator runner: %w", err)
	}

	return &Estimator{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "estimator",
		repo:           cfg.Repo,
		toolDeps:       deps,
	}, nil
}

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader.
func (e *Estimator) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	if e == nil || e.toolDeps == nil {
		return
	}
	e.toolDeps.SetOrganizationAISettingsReader(reader)
}

// SetCatalogReader injects the catalog reader (set after construction to break circular deps).
func (e *Estimator) SetCatalogReader(cr ports.CatalogReader) {
	e.toolDeps.CatalogReader = cr
}

// SetQuoteDrafter injects the quote drafter (set after construction to break circular deps).
func (e *Estimator) SetQuoteDrafter(qd ports.QuoteDrafter) {
	e.toolDeps.QuoteDrafter = qd
}

// Run executes estimation for a lead service.
func (e *Estimator) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	e.runMu.Lock()
	defer e.runMu.Unlock()

	e.toolDeps.SetTenantID(tenantID)
	e.toolDeps.SetLeadContext(leadID, serviceID)
	e.toolDeps.SetActor(repository.ActorTypeAI, repository.ActorNameEstimator)
	e.toolDeps.ResetToolCallTracking()
	runID := e.toolDeps.GetRunID()
	log.Printf("estimator: run started runID=%s lead=%s service=%s tenant=%s", runID, leadID, serviceID, tenantID)

	// Preload org AI settings so ListCatalogGaps defaults reflect the tenant configuration.
	if _, err := e.toolDeps.LoadOrganizationAISettings(ctx); err != nil {
		log.Printf("estimator: failed to load org AI settings (tenant=%s): %v", tenantID, err)
	}

	existingQuoteID, quoteLookupErr := e.repo.GetLatestDraftQuoteID(ctx, serviceID, tenantID)
	if quoteLookupErr != nil {
		log.Printf("estimator: failed to lookup existing draft quote for service=%s: %v", serviceID, quoteLookupErr)
		e.toolDeps.SetExistingQuoteID(nil)
	} else {
		e.toolDeps.SetExistingQuoteID(existingQuoteID)
	}

	lead, err := e.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := e.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	notes, err := e.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		log.Printf("estimator notes fetch failed: %v", err)
		notes = nil
	}

	var photo *repository.PhotoAnalysis
	if analysis, err := e.repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID); err == nil {
		photo = &analysis
	}

	estimationContext := e.fetchEstimationGuidelines(ctx, tenantID, service.ServiceType)

	promptText := buildEstimatorPrompt(lead, service, notes, photo, estimationContext)
	if err := e.runWithPrompt(ctx, promptText, leadID); err != nil {
		return err
	}

	if !e.toolDeps.WasSaveEstimationCalled() {
		warn := "Estimator heeft geen schatting opgeslagen. Handmatige controle vereist."
		_, _ = e.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
			LeadID:         leadID,
			ServiceID:      &serviceID,
			OrganizationID: tenantID,
			ActorType:      repository.ActorTypeSystem,
			ActorName:      repository.ActorNameEstimator,
			EventType:      repository.EventTypeAlert,
			Title:          repository.EventTitleEstimationMissing,
			Summary:        &warn,
		})
		log.Printf("estimator: SaveEstimation was not called for lead=%s service=%s", leadID, serviceID)
	}

	insufficientIntake := false
	insufficientReason := ""
	if !e.toolDeps.WasDraftQuoteCalled() {
		if insufficient, reason := e.hasInsufficientIntakeForDraft(ctx, serviceID, tenantID); insufficient {
			insufficientIntake = true
			insufficientReason = reason
			summary := "Onvoldoende intakegegevens voor een betrouwbare conceptofferte. Vraag aanvullende metingen/details op voordat de offerte wordt opgesteld."
			_, _ = e.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
				LeadID:         leadID,
				ServiceID:      &serviceID,
				OrganizationID: tenantID,
				ActorType:      repository.ActorTypeSystem,
				ActorName:      repository.ActorNameEstimator,
				EventType:      repository.EventTypeAlert,
				Title:          repository.EventTitleEstimationMissing,
				Summary:        &summary,
				Metadata: repository.AlertMetadata{
					Trigger: reason,
				}.ToMap(),
			})
			log.Printf("estimator: DraftQuote skipped due to insufficient intake for lead=%s service=%s reason=%s", leadID, serviceID, reason)
		} else {
			log.Printf("estimator: DraftQuote was not called for lead=%s service=%s", leadID, serviceID)
		}
	}

	if insufficientIntake && !e.toolDeps.WasStageUpdateCalled() {
		reason := "Onvoldoende intakegegevens voor betrouwbare conceptofferte; aanvullende metingen nodig."
		currentService := service
		if latestService, loadErr := e.repo.GetLeadServiceByID(ctx, serviceID, tenantID); loadErr == nil {
			currentService = latestService
		}
		if currentService.PipelineStage == domain.PipelineStageNurturing {
			log.Printf("estimator: skipping fallback stage update (already Nurturing) runID=%s lead=%s service=%s", runID, leadID, serviceID)
		} else if _, err := e.repo.UpdatePipelineStage(ctx, serviceID, tenantID, domain.PipelineStageNurturing); err != nil {
			log.Printf("estimator: fallback stage update to Nurturing failed (runID=%s lead=%s service=%s): %v", runID, leadID, serviceID, err)
		} else {
			_, _ = e.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
				LeadID:         leadID,
				ServiceID:      &serviceID,
				OrganizationID: tenantID,
				ActorType:      repository.ActorTypeSystem,
				ActorName:      repository.ActorNameEstimator,
				EventType:      repository.EventTypeStageChange,
				Title:          repository.EventTitleStageUpdated,
				Summary:        &reason,
				Metadata: repository.StageChangeMetadata{
					OldStage: currentService.PipelineStage,
					NewStage: domain.PipelineStageNurturing,
				}.ToMap(),
			})
			log.Printf("estimator: applied fallback stage update to Nurturing (runID=%s lead=%s service=%s reason=%s)", runID, leadID, serviceID, insufficientReason)
		}
	}
	log.Printf("estimator: run finished runID=%s lead=%s service=%s", runID, leadID, serviceID)

	return nil
}

// fetchEstimationGuidelines returns the estimation guidelines for the given
// service type, or an empty string when none are configured.
func (e *Estimator) fetchEstimationGuidelines(ctx context.Context, tenantID uuid.UUID, serviceType string) string {
	serviceTypes, err := e.repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return ""
	}
	for _, st := range serviceTypes {
		if st.Name == serviceType && st.EstimationGuidelines != nil {
			return *st.EstimationGuidelines
		}
	}
	return ""
}

func (e *Estimator) runWithPrompt(ctx context.Context, promptText string, leadID uuid.UUID) error {
	sessionID := uuid.New().String()
	userID := "estimator-" + leadID.String()

	_, err := e.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   e.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create estimator session: %w", err)
	}
	defer func() {
		_ = e.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   e.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	for event := range e.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		_ = event
	}

	return nil
}

func (e *Estimator) hasInsufficientIntakeForDraft(ctx context.Context, serviceID, tenantID uuid.UUID) (bool, string) {
	analysis, err := e.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		return true, "gatekeeper_analysis_unavailable"
	}
	if strings.EqualFold(strings.TrimSpace(analysis.RecommendedAction), "RequestInfo") {
		return true, "gatekeeper_request_info"
	}
	if domain.HasNonEmptyMissingInformation(analysis.MissingInformation) {
		return true, "gatekeeper_missing_information"
	}
	return false, ""
}
