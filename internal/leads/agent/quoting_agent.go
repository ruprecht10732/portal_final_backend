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

// GenerateResult is the return value from a prompt-based quote generation.
type GenerateResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

// QuotingAgent unifies autonomous estimation and prompt-driven quote generation.
type QuotingAgent struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	runMu          sync.Mutex
	autonomous     bool
}

// QuotingAgentConfig holds shared dependencies for both quoting modes.
type QuotingAgentConfig struct {
	APIKey               string
	Repo                 repository.LeadsRepository
	EventBus             events.Bus
	EmbeddingClient      *embeddings.Client
	QdrantClient         *qdrant.Client
	BouwmaatQdrantClient *qdrant.Client
	CatalogQdrantClient  *qdrant.Client
	CatalogReader        ports.CatalogReader
	QuoteDrafter         ports.QuoteDrafter
	IsAutonomous         bool
}

// NewQuotingAgent creates a quoting agent in autonomous (estimator) or prompt mode.
func NewQuotingAgent(cfg QuotingAgentConfig) (*QuotingAgent, error) {
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

	tools, err := buildQuotingTools(deps, cfg.IsAutonomous)
	if err != nil {
		return nil, err
	}

	name := "QuoteGenerator"
	description := "Generates draft quotes from a user prompt using catalog search."
	instruction := "You are a Quote Generator. Search for products and create draft quotes."
	appName := "quote-generator"
	if cfg.IsAutonomous {
		name = "Estimator"
		description = "Technical estimator that scopes work and suggests price ranges."
		instruction = "You are a Technical Estimator."
		appName = "estimator"
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        name,
		Model:       kimi,
		Description: description,
		Instruction: instruction,
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create quoting agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create quoting agent runner: %w", err)
	}

	return &QuotingAgent{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        appName,
		repo:           cfg.Repo,
		toolDeps:       deps,
		autonomous:     cfg.IsAutonomous,
	}, nil
}

func buildQuotingTools(deps *ToolDependencies, autonomous bool) ([]tool.Tool, error) {
	calculatorTool, err := createCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build Calculator tool: %w", err)
	}

	draftQuoteTool, err := createDraftQuoteTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build DraftQuote tool: %w", err)
	}

	tools := []tool.Tool{calculatorTool, draftQuoteTool}

	if autonomous {
		calculateEstimateTool, err := createCalculateEstimateTool()
		if err != nil {
			return nil, fmt.Errorf("failed to build CalculateEstimate tool: %w", err)
		}

		saveEstimationTool, err := createSaveEstimationTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build SaveEstimation tool: %w", err)
		}

		updateStageTool, err := createUpdatePipelineStageTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build UpdatePipelineStage tool: %w", err)
		}

		listCatalogGapsTool, err := createListCatalogGapsTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build ListCatalogGaps tool: %w", err)
		}

		tools = append(tools, calculateEstimateTool, saveEstimationTool, updateStageTool, listCatalogGapsTool)
	}

	if deps.IsProductSearchEnabled() {
		searchTool, err := createSearchProductMaterialsTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build SearchProductMaterials tool: %w", err)
		}
		tools = append(tools, searchTool)
		log.Printf("QuotingAgent: product search enabled (autonomous=%t)", autonomous)
	} else {
		log.Printf("QuotingAgent: product search disabled (autonomous=%t)", autonomous)
	}

	return tools, nil
}

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader.
func (q *QuotingAgent) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	if q == nil || q.toolDeps == nil {
		return
	}
	q.toolDeps.SetOrganizationAISettingsReader(reader)
}

// SetCatalogReader injects the catalog reader (set after construction to break circular deps).
func (q *QuotingAgent) SetCatalogReader(cr ports.CatalogReader) {
	q.toolDeps.CatalogReader = cr
}

// SetQuoteDrafter injects the quote drafter (set after construction to break circular deps).
func (q *QuotingAgent) SetQuoteDrafter(qd ports.QuoteDrafter) {
	q.toolDeps.QuoteDrafter = qd
}

// Run executes autonomous estimation for a lead service.
func (q *QuotingAgent) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error {
	if !q.autonomous {
		return fmt.Errorf("quoting agent is not configured for autonomous runs")
	}
	log.Printf("quoting-agent: scheduling autonomous run for lead=%s service=%s tenant=%s force=%t", leadID, serviceID, tenantID, force)
	go q.runEstimation(ctx, leadID, serviceID, tenantID, force)
	return nil
}

func (q *QuotingAgent) runEstimation(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) {
	q.runMu.Lock()
	defer q.runMu.Unlock()

	runID := q.startAutonomousRun(ctx, leadID, serviceID, tenantID)
	lead, service, notes, photo, ok := q.loadAutonomousRunContext(ctx, leadID, serviceID, tenantID)
	if !ok {
		return
	}

	if !q.executeAutonomousPrompt(ctx, lead, service, notes, photo, tenantID) {
		return
	}

	q.maybeRecordMissingEstimation(ctx, leadID, serviceID, tenantID)
	insufficientIntake, insufficientReason := q.evaluateDraftReadiness(ctx, leadID, serviceID, tenantID, force)
	q.maybeApplyNurturingFallback(ctx, nurturingFallbackContext{
		LeadID:             leadID,
		ServiceID:          serviceID,
		TenantID:           tenantID,
		RunID:              runID,
		Service:            service,
		InsufficientIntake: insufficientIntake,
		InsufficientReason: insufficientReason,
	})

	log.Printf("quoting-agent: autonomous run finished runID=%s lead=%s service=%s", runID, leadID, serviceID)
}

type nurturingFallbackContext struct {
	LeadID             uuid.UUID
	ServiceID          uuid.UUID
	TenantID           uuid.UUID
	RunID              string
	Service            repository.LeadService
	InsufficientIntake bool
	InsufficientReason string
}

func (q *QuotingAgent) startAutonomousRun(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) string {
	q.toolDeps.SetTenantID(tenantID)
	q.toolDeps.SetLeadContext(leadID, serviceID)
	q.toolDeps.SetActor(repository.ActorTypeAI, repository.ActorNameEstimator)
	q.toolDeps.ResetToolCallTracking()
	runID := q.toolDeps.GetRunID()
	log.Printf("quoting-agent: autonomous run started runID=%s lead=%s service=%s tenant=%s", runID, leadID, serviceID, tenantID)

	if _, err := q.toolDeps.LoadOrganizationAISettings(ctx); err != nil {
		log.Printf("quoting-agent: failed to load org AI settings (tenant=%s): %v", tenantID, err)
	}

	existingQuoteID, quoteLookupErr := q.repo.GetLatestDraftQuoteID(ctx, serviceID, tenantID)
	if quoteLookupErr != nil {
		log.Printf("quoting-agent: failed to lookup existing draft quote for service=%s: %v", serviceID, quoteLookupErr)
		q.toolDeps.SetExistingQuoteID(nil)
	} else {
		q.toolDeps.SetExistingQuoteID(existingQuoteID)
	}

	return runID
}

func (q *QuotingAgent) loadAutonomousRunContext(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) (repository.Lead, repository.LeadService, []repository.LeadNote, *repository.PhotoAnalysis, bool) {
	lead, err := q.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		log.Printf("quoting-agent: failed to get lead by id: %v", err)
		return repository.Lead{}, repository.LeadService{}, nil, nil, false
	}

	service, err := q.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		log.Printf("quoting-agent: failed to get lead service by id: %v", err)
		return repository.Lead{}, repository.LeadService{}, nil, nil, false
	}

	notes, err := q.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		log.Printf("quoting-agent: notes fetch failed: %v", err)
		notes = nil
	}

	var photo *repository.PhotoAnalysis
	if analysis, err := q.repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID); err == nil {
		photo = &analysis
	}

	return lead, service, notes, photo, true
}

func (q *QuotingAgent) executeAutonomousPrompt(ctx context.Context, lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photo *repository.PhotoAnalysis, tenantID uuid.UUID) bool {
	estimationContext := q.fetchEstimationGuidelines(ctx, tenantID, service.ServiceType)
	promptText := buildEstimatorPrompt(lead, service, notes, photo, estimationContext)
	if err := q.runWithPrompt(ctx, promptText, "estimator-"+lead.ID.String()); err != nil {
		log.Printf("quoting-agent: error from autonomous runWithPrompt: %v", err)
		return false
	}
	return true
}

func (q *QuotingAgent) maybeRecordMissingEstimation(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) {
	if q.toolDeps.WasSaveEstimationCalled() {
		return
	}

	warn := "Estimator heeft geen schatting opgeslagen. Handmatige controle vereist."
	_, _ = q.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameEstimator,
		EventType:      repository.EventTypeAlert,
		Title:          repository.EventTitleEstimationMissing,
		Summary:        &warn,
	})
	log.Printf("quoting-agent: SaveEstimation was not called for lead=%s service=%s", leadID, serviceID)
}

func (q *QuotingAgent) evaluateDraftReadiness(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) (bool, string) {
	if q.toolDeps.WasDraftQuoteCalled() || force {
		return false, ""
	}

	insufficient, reason := q.hasInsufficientIntakeForDraft(ctx, serviceID, tenantID)
	if !insufficient {
		log.Printf("quoting-agent: DraftQuote was not called for lead=%s service=%s", leadID, serviceID)
		return false, ""
	}

	summary := "Onvoldoende intakegegevens voor een betrouwbare conceptofferte. Vraag aanvullende metingen/details op voordat de offerte wordt opgesteld."
	_, _ = q.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
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
	log.Printf("quoting-agent: DraftQuote skipped due to insufficient intake for lead=%s service=%s reason=%s", leadID, serviceID, reason)

	return true, reason
}

func (q *QuotingAgent) maybeApplyNurturingFallback(ctx context.Context, fallback nurturingFallbackContext) {
	if !fallback.InsufficientIntake || q.toolDeps.WasStageUpdateCalled() {
		return
	}

	reason := "Onvoldoende intakegegevens voor betrouwbare conceptofferte; aanvullende metingen nodig."
	currentService := fallback.Service
	if latestService, loadErr := q.repo.GetLeadServiceByID(ctx, fallback.ServiceID, fallback.TenantID); loadErr == nil {
		currentService = latestService
	}

	if currentService.PipelineStage == domain.PipelineStageNurturing {
		log.Printf("quoting-agent: skipping fallback stage update (already Nurturing) runID=%s lead=%s service=%s", fallback.RunID, fallback.LeadID, fallback.ServiceID)
		return
	}

	if _, err := q.repo.UpdatePipelineStage(ctx, fallback.ServiceID, fallback.TenantID, domain.PipelineStageNurturing); err != nil {
		log.Printf("quoting-agent: fallback stage update to Nurturing failed (runID=%s lead=%s service=%s): %v", fallback.RunID, fallback.LeadID, fallback.ServiceID, err)
		return
	}

	_, _ = q.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         fallback.LeadID,
		ServiceID:      &fallback.ServiceID,
		OrganizationID: fallback.TenantID,
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
	log.Printf("quoting-agent: applied fallback stage update to Nurturing (runID=%s lead=%s service=%s reason=%s)", fallback.RunID, fallback.LeadID, fallback.ServiceID, fallback.InsufficientReason)
}

// Generate runs prompt-driven quote generation.
func (q *QuotingAgent) Generate(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userPrompt string, existingQuoteID *uuid.UUID, force bool) (*GenerateResult, error) {
	q.runMu.Lock()
	defer q.runMu.Unlock()

	q.toolDeps.SetTenantID(tenantID)
	q.toolDeps.SetLeadContext(leadID, serviceID)
	q.toolDeps.SetActor("AI", "Quote Generator")
	q.toolDeps.ResetToolCallTracking()
	q.toolDeps.SetExistingQuoteID(existingQuoteID)
	q.toolDeps.SetForceDraftQuote(force)

	if _, err := q.toolDeps.LoadOrganizationAISettings(ctx); err != nil {
		log.Printf("quoting-agent: failed to load org AI settings (tenant=%s): %v", tenantID, err)
	}

	lead, err := q.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quote generator: load lead: %w", err)
	}
	service, err := q.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quote generator: load service: %w", err)
	}

	notes, err := q.repo.ListNotesByService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		log.Printf("quoting-agent: notes fetch failed: %v", err)
		notes = nil
	}

	estimationContext := q.fetchEstimationGuidelines(ctx, tenantID, service.ServiceType)
	promptText := buildQuoteGeneratePrompt(lead, service, notes, userPrompt, estimationContext)

	if err := q.runWithPrompt(ctx, promptText, "quote-gen-"+leadID.String()); err != nil {
		return nil, fmt.Errorf("quote generator: run failed: %w", err)
	}

	result := q.toolDeps.GetLastDraftResult()
	if result == nil {
		return nil, fmt.Errorf("quote generator: agent did not produce a draft quote")
	}

	return &GenerateResult{
		QuoteID:     result.QuoteID,
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func (q *QuotingAgent) fetchEstimationGuidelines(ctx context.Context, tenantID uuid.UUID, serviceType string) string {
	serviceTypes, err := q.repo.ListActiveServiceTypes(ctx, tenantID)
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

func (q *QuotingAgent) runWithPrompt(ctx context.Context, promptText, userID string) error {
	sessionID := uuid.New().String()

	_, err := q.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   q.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create quoting session: %w", err)
	}
	defer func() {
		_ = q.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   q.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	for event := range q.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		_ = event
	}

	return nil
}

func (q *QuotingAgent) hasInsufficientIntakeForDraft(ctx context.Context, serviceID, tenantID uuid.UUID) (bool, string) {
	analysis, err := q.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
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
