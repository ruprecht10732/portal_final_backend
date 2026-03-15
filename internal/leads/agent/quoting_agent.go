package agent

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

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
	"portal_final_backend/internal/orchestration"
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

// QuotingAgentDependencies captures shared dependency injection hooks.
type QuotingAgentDependencies interface {
	SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader)
	SetCatalogReader(cr ports.CatalogReader)
	SetQuoteDrafter(qd ports.QuoteDrafter)
	SetPricingIntelligenceReader(reader ports.PricingIntelligenceReader)
}

// Estimator exposes the autonomous estimation workflow surface.
type Estimator interface {
	QuotingAgentDependencies
	Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error
	Execute(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error
}

// QuoteGenerator exposes the prompt-driven quote generation surface.
type QuoteGenerator interface {
	QuotingAgentDependencies
	Generate(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userPrompt string, existingQuoteID *uuid.UUID, force bool) (*GenerateResult, error)
}

var (
	_ Estimator      = (*QuotingAgent)(nil)
	_ QuoteGenerator = (*QuotingAgent)(nil)
)

// QuotingAgent unifies autonomous estimation and prompt-driven quote generation.
type QuotingAgent struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	modelConfig    moonshot.Config
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	mode           quotingAgentMode
}

type quotingAgentMode string

const (
	quotingAgentModeEstimator      quotingAgentMode = "estimator"
	quotingAgentModeQuoteGenerator quotingAgentMode = "quote-generator"
	maxQuoteRepairAttempts         int              = 2
)

type quoteCriticLoopReview struct {
	result   *ports.QuoteAIReviewResult
	critique *SubmitQuoteCritiqueInput
}

type quoteCriticContext struct {
	lead     repository.Lead
	service  repository.LeadService
	notes    []repository.LeadNote
	photo    *repository.PhotoAnalysis
	tenantID uuid.UUID
}

type quotingAgentProfile struct {
	name        string
	description string
	workspace   string
	appName     string
	reasoning   bool
}

// QuotingAgentConfig holds shared dependencies for both quoting modes.
type QuotingAgentConfig struct {
	APIKey               string
	Model                string
	Repo                 repository.LeadsRepository
	EventBus             events.Bus
	EmbeddingClient      *embeddings.Client
	QdrantClient         *qdrant.Client
	BouwmaatQdrantClient *qdrant.Client
	CatalogQdrantClient  *qdrant.Client
	CatalogReader        ports.CatalogReader
	QuoteDrafter         ports.QuoteDrafter
	PricingIntelligence  ports.PricingIntelligenceReader
}

// NewEstimatorAgent creates the autonomous estimator agent.
func NewEstimatorAgent(cfg QuotingAgentConfig) (*QuotingAgent, error) {
	return newQuotingAgent(cfg, quotingAgentModeEstimator)
}

// NewQuoteGeneratorAgent creates the prompt-driven quote generator agent.
func NewQuoteGeneratorAgent(cfg QuotingAgentConfig) (*QuotingAgent, error) {
	return newQuotingAgent(cfg, quotingAgentModeQuoteGenerator)
}

func newQuotingAgent(cfg QuotingAgentConfig, mode quotingAgentMode) (*QuotingAgent, error) {
	profile := mode.profile()
	modelConfig := newMoonshotModelConfig(cfg.APIKey, cfg.Model)
	if profile.reasoning {
		modelConfig = newMoonshotReasoningModelConfig(cfg.APIKey, cfg.Model)
	}
	kimi := moonshot.NewModel(modelConfig)
	workspace, err := orchestration.LoadAgentWorkspace(profile.workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s workspace context: %w", profile.workspace, err)
	}

	deps := &ToolDependencies{
		Repo:                 cfg.Repo,
		EventBus:             cfg.EventBus,
		EmbeddingClient:      cfg.EmbeddingClient,
		QdrantClient:         cfg.QdrantClient,
		BouwmaatQdrantClient: cfg.BouwmaatQdrantClient,
		CatalogQdrantClient:  cfg.CatalogQdrantClient,
		CatalogReader:        cfg.CatalogReader,
		QuoteDrafter:         cfg.QuoteDrafter,
		PricingIntelligence:  cfg.PricingIntelligence,
		CouncilService:       NewDefaultMultiAgentCouncil(cfg.Repo),
	}

	tools, err := buildQuotingTools(deps, mode)
	if err != nil {
		return nil, err
	}
	toolsets := orchestration.BuildWorkspaceToolsets(workspace, profile.appName+"_tools", tools)

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        profile.name,
		Model:       kimi,
		Description: profile.description,
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s agent: %w", mode, err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        profile.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s runner: %w", mode, err)
	}

	return &QuotingAgent{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        profile.appName,
		modelConfig:    modelConfig,
		repo:           cfg.Repo,
		toolDeps:       deps,
		mode:           mode,
	}, nil
}

func (m quotingAgentMode) isAutonomous() bool {
	return m == quotingAgentModeEstimator
}

func (m quotingAgentMode) profile() quotingAgentProfile {
	switch m {
	case quotingAgentModeEstimator:
		return quotingAgentProfile{
			name:        "Estimator",
			description: "Technical estimator that scopes work and suggests price ranges.",
			workspace:   "calculator",
			appName:     "estimator",
			reasoning:   true,
		}
	default:
		return quotingAgentProfile{
			name:        "QuoteGenerator",
			description: "Generates draft quotes from a user prompt using catalog search.",
			workspace:   "calculator",
			appName:     "quote-generator",
			reasoning:   true,
		}
	}
}

func buildQuotingTools(deps *ToolDependencies, mode quotingAgentMode) ([]tool.Tool, error) {
	calculatorTool, err := createCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build Calculator tool: %w", err)
	}

	draftQuoteTool, err := createDraftQuoteTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build DraftQuote tool: %w", err)
	}

	tools := []tool.Tool{calculatorTool, draftQuoteTool}

	if mode.isAutonomous() {
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
		log.Printf("QuotingAgent[%s]: product search enabled", mode)
	} else {
		log.Printf("QuotingAgent[%s]: product search disabled", mode)
	}

	return tools, nil
}

func (q *QuotingAgent) buildScopeAnalyzerTools() ([]tool.Tool, error) {
	commitScopeTool, err := createCommitScopeArtifactTool(q.toolDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to build CommitScopeArtifact tool: %w", err)
	}
	return []tool.Tool{commitScopeTool}, nil
}

func (q *QuotingAgent) buildInvestigativeTools() ([]tool.Tool, error) {
	askClarificationTool, err := createAskCustomerClarificationTool(q.toolDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to build AskCustomerClarification tool: %w", err)
	}
	return []tool.Tool{askClarificationTool}, nil
}

func (q *QuotingAgent) buildQuoteCriticTools() ([]tool.Tool, error) {
	criticTool, err := createSubmitQuoteCritiqueTool(q.toolDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to build SubmitQuoteCritique tool: %w", err)
	}
	return []tool.Tool{criticTool}, nil
}

func (q *QuotingAgent) buildQuoteRepairTools() ([]tool.Tool, error) {
	calculatorTool, err := createCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build Calculator tool: %w", err)
	}

	draftQuoteTool, err := createDraftQuoteTool(q.toolDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to build DraftQuote tool: %w", err)
	}

	calculateEstimateTool, err := createCalculateEstimateTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build CalculateEstimate tool: %w", err)
	}

	tools := []tool.Tool{calculatorTool, draftQuoteTool, calculateEstimateTool}
	if q.toolDeps.IsProductSearchEnabled() {
		searchTool, searchErr := createSearchProductMaterialsTool(q.toolDeps)
		if searchErr != nil {
			return nil, fmt.Errorf("failed to build SearchProductMaterials tool: %w", searchErr)
		}
		tools = append(tools, searchTool)
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

// SetPricingIntelligenceReader injects quote pricing intelligence retrieval.
func (q *QuotingAgent) SetPricingIntelligenceReader(reader ports.PricingIntelligenceReader) {
	q.toolDeps.PricingIntelligence = reader
}

// Run executes autonomous estimation for a lead service.
func (q *QuotingAgent) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error {
	if !q.mode.isAutonomous() {
		return fmt.Errorf("quoting agent is not configured for autonomous runs")
	}
	log.Printf("quoting-agent[%s]: scheduling run for lead=%s service=%s tenant=%s force=%t", q.mode, leadID, serviceID, tenantID, force)
	reqDeps := q.toolDeps.NewRequestDeps()
	ctx = WithDependencies(ctx, reqDeps)
	go func() {
		if err := q.executeAutonomousRun(ctx, reqDeps, leadID, serviceID, tenantID, force); err != nil {
			log.Printf("quoting-agent[%s]: autonomous run failed lead=%s service=%s tenant=%s: %v", q.mode, leadID, serviceID, tenantID, err)
		}
	}()
	return nil
}

func (q *QuotingAgent) Execute(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error {
	if !q.mode.isAutonomous() {
		return fmt.Errorf("quoting agent is not configured for autonomous runs")
	}

	reqDeps := q.toolDeps.NewRequestDeps()
	ctx = WithDependencies(ctx, reqDeps)
	return q.executeAutonomousRun(ctx, reqDeps, leadID, serviceID, tenantID, force)
}

func (q *QuotingAgent) executeAutonomousRun(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, force bool) error {
	runID := q.startAutonomousRun(ctx, reqDeps, leadID, serviceID, tenantID)
	lead, service, notes, photo, ok := q.loadAutonomousRunContext(ctx, leadID, serviceID, tenantID)
	if !ok {
		return nil
	}

	if !q.executeAutonomousPrompt(ctx, lead, service, notes, photo, tenantID) {
		return nil
	}

	q.runQuoteCriticAndRepair(ctx, reqDeps, lead, service, notes, photo, tenantID)

	if !reqDeps.WasClarificationAsked() {
		q.maybeRecordMissingEstimation(ctx, reqDeps, leadID, serviceID, tenantID)
	}
	insufficientIntake, insufficientReason := q.evaluateDraftReadiness(ctx, reqDeps, leadID, serviceID, tenantID, force)
	q.maybeApplyNurturingFallback(ctx, reqDeps, nurturingFallbackContext{
		LeadID:             leadID,
		ServiceID:          serviceID,
		TenantID:           tenantID,
		RunID:              runID,
		Service:            service,
		InsufficientIntake: insufficientIntake,
		InsufficientReason: insufficientReason,
	})
	q.persistEstimatorDecisionMemory(ctx, reqDeps, leadID, serviceID, tenantID, service)

	log.Printf("quoting-agent[%s]: run finished runID=%s lead=%s service=%s", q.mode, runID, leadID, serviceID)
	return nil
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

func (q *QuotingAgent) startAutonomousRun(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID) string {
	reqDeps.SetTenantID(tenantID)
	reqDeps.SetLeadContext(leadID, serviceID)
	reqDeps.SetActor(repository.ActorTypeAI, repository.ActorNameEstimator)
	reqDeps.ResetToolCallTracking()
	runID := reqDeps.GetRunID()
	log.Printf("quoting-agent[%s]: run started runID=%s lead=%s service=%s tenant=%s", q.mode, runID, leadID, serviceID, tenantID)

	if _, err := reqDeps.LoadOrganizationAISettings(ctx); err != nil {
		log.Printf("quoting-agent: failed to load org AI settings (tenant=%s): %v", tenantID, err)
	}

	existingQuoteID, quoteLookupErr := q.repo.GetLatestDraftQuoteID(ctx, serviceID, tenantID)
	if quoteLookupErr != nil {
		log.Printf("quoting-agent: failed to lookup existing draft quote for service=%s: %v", serviceID, quoteLookupErr)
		reqDeps.SetExistingQuoteID(nil)
	} else {
		reqDeps.SetExistingQuoteID(existingQuoteID)
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
	reasoningMode, enrichedContext := q.buildEnhancedEstimationContext(ctx, tenantID, lead, service, notes, photo, estimationContext)

	if isInvestigativeMode(reasoningMode) {
		investigativeTools, err := q.buildInvestigativeTools()
		if err != nil {
			log.Printf("quoting-agent: failed to build investigative tools: %v", err)
			return false
		}

		missing := make([]string, 0)
		if analysis, err := q.repo.GetLatestAIAnalysis(ctx, service.ID, tenantID); err == nil {
			missing = append(missing, analysis.MissingInformation...)
		}

		promptText := buildInvestigativePrompt(lead, service, notes, photo, missing, enrichedContext)
		if err := q.runWithPromptUsingTools(ctx, promptText, "estimator-investigative-"+lead.ID.String(), "EstimatorInvestigative", "Investigative intake clarification mode", investigativeTools); err != nil {
			log.Printf("quoting-agent: error from investigative mode run: %v", err)
			return false
		}
		return true
	}

	scopeTools, err := q.buildScopeAnalyzerTools()
	if err != nil {
		log.Printf("quoting-agent: failed to build scope analyzer tools: %v", err)
		return false
	}

	scopePrompt := buildScopeAnalyzerPrompt(lead, service, notes, photo)
	if err := q.runWithPromptUsingTools(ctx, scopePrompt, "estimator-scope-"+lead.ID.String(), "ScopeAnalyzer", "Analyzes scope and commits artifact", scopeTools); err != nil {
		log.Printf("quoting-agent: error from scope analyzer run: %v", err)
		return false
	}

	scopeArtifact, ok := GetDependencies(ctx).GetScopeArtifact()
	if !ok {
		log.Printf("quoting-agent: scope analyzer did not commit artifact for lead=%s service=%s", lead.ID, service.ID)
		return false
	}

	quoteBuilderTools, err := buildQuotingTools(GetDependencies(ctx), q.mode)
	if err != nil {
		log.Printf("quoting-agent: failed to build quote builder tools: %v", err)
		return false
	}

	promptText := buildQuoteBuilderPrompt(lead, service, notes, photo, enrichedContext, scopeArtifact)
	if err := q.runWithPromptUsingTools(ctx, promptText, "estimator-quote-"+lead.ID.String(), "QuoteBuilder", "Builds estimate and draft quote from scope artifact", quoteBuilderTools); err != nil {
		log.Printf("quoting-agent: error from autonomous runWithPrompt: %v", err)
		return false
	}
	return true
}

func (q *QuotingAgent) buildEnhancedEstimationContext(ctx context.Context, tenantID uuid.UUID, lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photo *repository.PhotoAnalysis, baseGuidelines string) (estimatorReasoningMode, string) {
	settings := GetDependencies(ctx).GetOrganizationAISettingsOrDefault()
	latestAnalysis := q.loadLatestAnalysis(ctx, tenantID, service.ID)
	reasoningMode := chooseEstimatorReasoningMode(settings, latestAnalysis, photo)
	councilAdvice := q.resolveEstimatorCouncilAdvice(settings, latestAnalysis, photo, notes)
	memorySection := q.loadExperienceMemorySection(ctx, settings, tenantID, service.ServiceType)
	humanFeedbackSection := q.loadHumanFeedbackSection(ctx, settings, tenantID, service.ServiceType)
	pricingIntelligenceSection := q.loadPricingIntelligenceSection(ctx, settings, tenantID, service.ServiceType, derivePostcodePrefixZIP4(lead.AddressZipCode))

	var sb strings.Builder
	sb.WriteString(baseGuidelines)
	sb.WriteString("\n\n=== REASONING MODE ===\n")
	sb.WriteString("mode=")
	sb.WriteString(string(reasoningMode))
	sb.WriteString("\n")
	switch reasoningMode {
	case reasoningModeFast:
		sb.WriteString("Use concise reasoning and prioritize execution speed.\n")
	case reasoningModeDeliberate:
		sb.WriteString("Use careful, stepwise reasoning and enforce conservative assumptions.\n")
	default:
		sb.WriteString("Use balanced reasoning with explicit checks before final stage update.\n")
	}

	appendContextSection(&sb, memorySection)
	appendContextSection(&sb, humanFeedbackSection)
	appendContextSection(&sb, pricingIntelligenceSection)
	if settings.AICouncilMode {
		appendContextSection(&sb, buildCouncilSection(councilAdvice))
	}

	return reasoningMode, strings.TrimSpace(sb.String())
}

func (q *QuotingAgent) loadPricingIntelligenceSection(ctx context.Context, settings ports.OrganizationAISettings, tenantID uuid.UUID, serviceType string, postcodePrefix string) string {
	if !settings.AIExperienceMemory {
		return ""
	}
	deps := GetDependencies(ctx)
	if deps == nil || deps.PricingIntelligence == nil {
		return ""
	}
	report, err := deps.PricingIntelligence.GetPricingIntelligenceReport(ctx, tenantID, serviceType, postcodePrefix)
	if err != nil || report == nil {
		return ""
	}
	return buildPricingIntelligenceSection(report)
}

func (q *QuotingAgent) loadLatestAnalysis(ctx context.Context, tenantID, serviceID uuid.UUID) *repository.AIAnalysis {
	analysis, err := q.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		return nil
	}
	return &analysis
}

func (q *QuotingAgent) resolveEstimatorCouncilAdvice(settings ports.OrganizationAISettings, latestAnalysis *repository.AIAnalysis, photo *repository.PhotoAnalysis, notes []repository.LeadNote) estimatorCouncilAdvice {
	if !settings.AICouncilMode {
		return estimatorCouncilAdvice{}
	}
	return runEstimatorCouncil(latestAnalysis, photo, notes)
}

func (q *QuotingAgent) loadExperienceMemorySection(ctx context.Context, settings ports.OrganizationAISettings, tenantID uuid.UUID, serviceType string) string {
	if !settings.AIExperienceMemory {
		return ""
	}
	memories, err := q.repo.ListRecentAIDecisionMemories(ctx, tenantID, serviceType, 6)
	if err != nil {
		return ""
	}
	return buildExperienceMemorySection(memories)
}

func (q *QuotingAgent) loadHumanFeedbackSection(ctx context.Context, settings ports.OrganizationAISettings, tenantID uuid.UUID, serviceType string) string {
	if !settings.AIExperienceMemory {
		return ""
	}
	feedbackItems, err := q.repo.ListRecentAppliedHumanFeedbackByServiceType(ctx, tenantID, serviceType, 6)
	if err != nil {
		return ""
	}
	return buildHumanFeedbackMemorySection(feedbackItems)
}

func appendContextSection(sb *strings.Builder, section string) {
	if section == "" {
		return
	}
	sb.WriteString("\n")
	sb.WriteString(section)
	sb.WriteString("\n")
}

func (q *QuotingAgent) persistEstimatorDecisionMemory(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, service repository.LeadService) {
	settings := reqDeps.GetOrganizationAISettingsOrDefault()
	if !settings.AIExperienceMemory {
		return
	}

	updatedService, err := q.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return
	}

	analysis, analysisErr := q.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if analysisErr != nil {
		analysis = repository.AIAnalysis{}
	}

	outcome := "estimation_pending"
	if strings.EqualFold(updatedService.PipelineStage, domain.PipelineStageNurturing) {
		outcome = "nurturing_fallback"
	} else if reqDeps.WasDraftQuoteCalled() {
		outcome = "draft_quote_created"
	} else if reqDeps.WasSaveEstimationCalled() {
		outcome = "estimation_saved"
	}

	contextSummary := fmt.Sprintf("serviceType=%s, stage=%s, status=%s, missingInfo=%d", service.ServiceType, updatedService.PipelineStage, updatedService.Status, len(analysis.MissingInformation))
	actionSummary := fmt.Sprintf("draftQuote=%t, saveEstimation=%t, updateStage=%t", reqDeps.WasDraftQuoteCalled(), reqDeps.WasSaveEstimationCalled(), reqDeps.WasStageUpdateCalled())

	_, _ = q.repo.CreateAIDecisionMemory(ctx, repository.CreateAIDecisionMemoryParams{
		OrganizationID: tenantID,
		LeadID:         &leadID,
		LeadServiceID:  &serviceID,
		ServiceType:    service.ServiceType,
		DecisionType:   "estimator_run",
		Outcome:        outcome,
		Confidence:     analysis.CompositeConfidence,
		ContextSummary: contextSummary,
		ActionSummary:  actionSummary,
	})
}

func (q *QuotingAgent) maybeRecordMissingEstimation(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID) {
	if reqDeps.WasSaveEstimationCalled() {
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

func (q *QuotingAgent) evaluateDraftReadiness(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, force bool) (bool, string) {
	if reqDeps.WasDraftQuoteCalled() || force {
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

func (q *QuotingAgent) maybeApplyNurturingFallback(ctx context.Context, reqDeps *ToolDependencies, fallback nurturingFallbackContext) {
	if !fallback.InsufficientIntake || reqDeps.WasStageUpdateCalled() {
		return
	}

	reason := "Onvoldoende intakegegevens voor betrouwbare conceptofferte; aanvullende metingen nodig."
	currentService := fallback.Service
	if latestService, loadErr := q.repo.GetLeadServiceByID(ctx, fallback.ServiceID, fallback.TenantID); loadErr == nil {
		currentService = latestService
	}

	// If the stage changed since the run started, a human (or another process)
	// already acted — do not override their decision.
	if currentService.PipelineStage != fallback.Service.PipelineStage {
		log.Printf("quoting-agent: stage changed externally during run (was %s, now %s), skipping fallback runID=%s lead=%s service=%s",
			fallback.Service.PipelineStage, currentService.PipelineStage, fallback.RunID, fallback.LeadID, fallback.ServiceID)
		return
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
			RunID:    fallback.RunID,
		}.ToMap(),
	})
	log.Printf("quoting-agent: applied fallback stage update to Nurturing (runID=%s lead=%s service=%s reason=%s)", fallback.RunID, fallback.LeadID, fallback.ServiceID, fallback.InsufficientReason)
}

// Generate runs prompt-driven quote generation.
func (q *QuotingAgent) Generate(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userPrompt string, existingQuoteID *uuid.UUID, force bool) (*GenerateResult, error) {
	reqDeps := q.toolDeps.NewRequestDeps()
	reqDeps.SetTenantID(tenantID)
	reqDeps.SetLeadContext(leadID, serviceID)
	reqDeps.SetActor("AI", "Quote Generator")
	reqDeps.ResetToolCallTracking()
	reqDeps.SetExistingQuoteID(existingQuoteID)
	reqDeps.SetForceDraftQuote(force)

	ctx = WithDependencies(ctx, reqDeps)

	if _, err := reqDeps.LoadOrganizationAISettings(ctx); err != nil {
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

	result := reqDeps.GetLastDraftResult()
	if result == nil {
		return nil, fmt.Errorf("quote generator: agent did not produce a draft quote")
	}

	q.runQuoteCriticAndRepair(ctx, reqDeps, lead, service, notes, nil, tenantID)

	return &GenerateResult{
		QuoteID:     result.QuoteID,
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func (q *QuotingAgent) runQuoteCriticAndRepair(ctx context.Context, reqDeps *ToolDependencies, lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photo *repository.PhotoAnalysis, tenantID uuid.UUID) {
	criticCtx := quoteCriticContext{
		lead:     lead,
		service:  service,
		notes:    notes,
		photo:    photo,
		tenantID: tenantID,
	}

	runQuoteCriticRepairLoop(maxQuoteRepairAttempts,
		func(attempt int) (*quoteCriticLoopReview, bool) {
			reviewResult, ok := q.runQuoteCriticAttempt(ctx, reqDeps, criticCtx, attempt)
			if !ok || reviewResult == nil {
				return nil, ok
			}

			critiqueInput, hasCritique := reqDeps.GetLastQuoteCritiqueInput()
			if !hasCritique {
				return &quoteCriticLoopReview{result: reviewResult}, true
			}

			return &quoteCriticLoopReview{
				result:   reviewResult,
				critique: critiqueInput,
			}, true
		},
		func(attempt int) bool {
			return q.runQuoteRepairAttempt(ctx, reqDeps, criticCtx, attempt)
		},
		func(summary string) {
			q.emitQuoteCriticHumanAlert(ctx, reqDeps, criticCtx.lead.ID, criticCtx.service.ID, criticCtx.tenantID, summary)
		},
		func(summary string) {
			q.persistQuoteCriticExhausted(ctx, reqDeps, criticCtx.lead.ID, criticCtx.service.ID, criticCtx.tenantID, summary)
		},
		func(summary string) {
			q.persistQuoteCriticRepeatedFindings(ctx, reqDeps, criticCtx.lead.ID, criticCtx.service.ID, criticCtx.tenantID, summary)
		},
	)
}

func runQuoteCriticRepairLoop(
	maxRepairAttempts int,
	runCritic func(attempt int) (*quoteCriticLoopReview, bool),
	runRepair func(attempt int) bool,
	onRequiresHuman func(summary string),
	onExhausted func(summary string),
	onRepeatedFindings func(summary string),
) {
	var previousCritique *SubmitQuoteCritiqueInput

	for attempt := 1; attempt <= maxRepairAttempts+1; attempt++ {
		review, ok := runCritic(attempt)
		if !ok || review == nil || review.result == nil {
			return
		}

		switch review.result.Decision {
		case ports.QuoteAIReviewDecisionApproved:
			return
		case ports.QuoteAIReviewDecisionRequiresHuman:
			onRequiresHuman(review.result.Summary)
			return
		}

		if previousCritique != nil && review.critique != nil && quoteCritiquesEquivalent(*previousCritique, *review.critique) {
			onRepeatedFindings(review.result.Summary)
			return
		}

		if attempt > maxRepairAttempts {
			onExhausted(review.result.Summary)
			return
		}

		if !runRepair(attempt) {
			onExhausted("AI-herstel van de conceptofferte is mislukt; menselijke controle vereist.")
			return
		}

		if review.critique != nil {
			copyCritique := cloneSubmitQuoteCritiqueInput(*review.critique)
			previousCritique = &copyCritique
		}
	}
}

func quoteCritiquesEquivalent(previous, current SubmitQuoteCritiqueInput) bool {
	if previous.Approved != current.Approved {
		return false
	}

	previousFindings := canonicalQuoteCritiqueFindings(previous.Findings)
	currentFindings := canonicalQuoteCritiqueFindings(current.Findings)
	previousSignals := canonicalQuoteCritiqueSignals(previous.Signals)
	currentSignals := canonicalQuoteCritiqueSignals(current.Signals)

	if len(previousFindings) == 0 && len(previousSignals) == 0 {
		return false
	}
	if len(currentFindings) == 0 && len(currentSignals) == 0 {
		return false
	}

	return stringSlicesEqual(previousFindings, currentFindings) && stringSlicesEqual(previousSignals, currentSignals)
}

func cloneSubmitQuoteCritiqueInput(input SubmitQuoteCritiqueInput) SubmitQuoteCritiqueInput {
	return SubmitQuoteCritiqueInput{
		Approved: input.Approved,
		Summary:  input.Summary,
		Findings: append([]QuoteCritiqueFinding(nil), input.Findings...),
		Signals:  append([]string(nil), input.Signals...),
	}
}

func canonicalQuoteCritiqueFindings(findings []QuoteCritiqueFinding) []string {
	canonical := make([]string, 0, len(findings))
	for _, finding := range findings {
		itemIndex := "-"
		if finding.ItemIndex != nil {
			itemIndex = fmt.Sprintf("%d", *finding.ItemIndex)
		}
		canonical = append(canonical, strings.Join([]string{
			strings.ToLower(strings.TrimSpace(finding.Code)),
			strings.ToLower(strings.TrimSpace(finding.Message)),
			strings.ToLower(strings.TrimSpace(finding.Severity)),
			itemIndex,
		}, "|"))
	}
	sort.Strings(canonical)
	return canonical
}

func canonicalQuoteCritiqueSignals(signals []string) []string {
	canonical := make([]string, 0, len(signals))
	for _, signal := range signals {
		trimmed := strings.ToLower(strings.TrimSpace(signal))
		if trimmed == "" {
			continue
		}
		canonical = append(canonical, trimmed)
	}
	sort.Strings(canonical)
	return canonical
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (q *QuotingAgent) runQuoteCriticAttempt(ctx context.Context, reqDeps *ToolDependencies, criticCtx quoteCriticContext, attempt int) (*ports.QuoteAIReviewResult, bool) {
	draftResult := reqDeps.GetLastDraftResult()
	if draftResult == nil {
		return nil, false
	}
	draftInput, ok := reqDeps.GetLastDraftInput()
	if !ok {
		return nil, false
	}

	criticTools, err := q.buildQuoteCriticTools()
	if err != nil {
		log.Printf("quoting-agent: failed to build quote critic tools: %v", err)
		q.persistQuoteCriticFallback(ctx, reqDeps, draftResult.QuoteID, criticCtx.tenantID, "AI-review kon niet worden gestart; menselijke controle vereist.")
		return reqDeps.GetLastQuoteReviewResult(), false
	}

	estimationContext := q.fetchEstimationGuidelines(ctx, criticCtx.tenantID, criticCtx.service.ServiceType)
	scopeArtifact, _ := reqDeps.GetScopeArtifact()
	promptText := buildQuoteCriticPrompt(quotePromptInput{
		lead:              criticCtx.lead,
		service:           criticCtx.service,
		notes:             criticCtx.notes,
		photoAnalysis:     criticCtx.photo,
		estimationContext: estimationContext,
		scopeArtifact:     scopeArtifact,
	}, *draftInput, draftResult)
	reqDeps.SetQuoteCriticAttempt(attempt)
	if err := q.runWithPromptUsingTools(ctx, promptText, "quote-critic-"+criticCtx.lead.ID.String(), "QuoteCritic", "Reviews the latest drafted quote before approval queue entry", criticTools); err != nil {
		log.Printf("quoting-agent: quote critic run failed: %v", err)
		q.persistQuoteCriticFallback(ctx, reqDeps, draftResult.QuoteID, criticCtx.tenantID, "AI-review is mislukt; menselijke controle vereist.")
		return reqDeps.GetLastQuoteReviewResult(), false
	}

	if reqDeps.GetLastQuoteReviewResult() == nil {
		q.persistQuoteCriticFallback(ctx, reqDeps, draftResult.QuoteID, criticCtx.tenantID, "AI-review leverde geen besluit op; menselijke controle vereist.")
		return reqDeps.GetLastQuoteReviewResult(), false
	}

	return reqDeps.GetLastQuoteReviewResult(), true
}

func (q *QuotingAgent) runQuoteRepairAttempt(ctx context.Context, reqDeps *ToolDependencies, criticCtx quoteCriticContext, attempt int) bool {
	draftInput, ok := reqDeps.GetLastDraftInput()
	if !ok {
		return false
	}
	critiqueInput, ok := reqDeps.GetLastQuoteCritiqueInput()
	if !ok {
		return false
	}

	repairTools, err := q.buildQuoteRepairTools()
	if err != nil {
		log.Printf("quoting-agent: failed to build quote repair tools: %v", err)
		return false
	}

	estimationContext := q.fetchEstimationGuidelines(ctx, criticCtx.tenantID, criticCtx.service.ServiceType)
	scopeArtifact, _ := reqDeps.GetScopeArtifact()
	promptText := buildQuoteRepairPrompt(quotePromptInput{
		lead:              criticCtx.lead,
		service:           criticCtx.service,
		notes:             criticCtx.notes,
		photoAnalysis:     criticCtx.photo,
		estimationContext: estimationContext,
		scopeArtifact:     scopeArtifact,
	}, *draftInput, *critiqueInput, attempt)
	if err := q.runWithPromptUsingTools(ctx, promptText, fmt.Sprintf("quote-repair-%s-%d", criticCtx.lead.ID.String(), attempt), "QuoteRepairEstimator", "Repairs the persisted draft quote using critic findings", repairTools); err != nil {
		log.Printf("quoting-agent: quote repair run failed: %v", err)
		return false
	}

	return reqDeps.GetLastDraftResult() != nil
}

func (q *QuotingAgent) persistQuoteCriticFallback(ctx context.Context, reqDeps *ToolDependencies, quoteID, tenantID uuid.UUID, summary string) {
	if reqDeps == nil || reqDeps.QuoteDrafter == nil {
		return
	}
	runID := reqDeps.GetRunID()
	reviewerName := "QuoteCritic"
	modelName := "moonshot"
	attemptCount := reqDeps.GetQuoteCriticAttempt()
	if attemptCount <= 0 {
		attemptCount = 1
	}
	reviewResult, err := reqDeps.QuoteDrafter.RecordQuoteAIReview(ctx, ports.RecordQuoteAIReviewParams{
		QuoteID:        quoteID,
		OrganizationID: tenantID,
		Decision:       "requires_human",
		Summary:        summary,
		Signals:        []string{"critic_execution_failed"},
		AttemptCount:   attemptCount,
		RunID:          &runID,
		ReviewerName:   &reviewerName,
		ModelName:      &modelName,
	})
	if err != nil {
		log.Printf("quoting-agent: failed to persist quote critic fallback: %v", err)
		return
	}
	reqDeps.SetLastQuoteReviewResult(reviewResult)
}

func (q *QuotingAgent) persistQuoteCriticExhausted(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, previousSummary string) {
	draftResult := reqDeps.GetLastDraftResult()
	if draftResult == nil || reqDeps == nil || reqDeps.QuoteDrafter == nil {
		return
	}
	runID := reqDeps.GetRunID()
	reviewerName := "QuoteCritic"
	modelName := "moonshot"
	summary := "AI-review bleef fouten vinden na automatische herstelpogingen; menselijke controle vereist."
	if strings.TrimSpace(previousSummary) != "" {
		summary = summary + " Laatste beoordeling: " + strings.TrimSpace(previousSummary)
	}
	reviewResult, err := reqDeps.QuoteDrafter.RecordQuoteAIReview(ctx, ports.RecordQuoteAIReviewParams{
		QuoteID:        draftResult.QuoteID,
		OrganizationID: tenantID,
		Decision:       ports.QuoteAIReviewDecisionRequiresHuman,
		Summary:        summary,
		Signals:        []string{"repair_attempts_exhausted"},
		AttemptCount:   maxQuoteRepairAttempts + 1,
		RunID:          &runID,
		ReviewerName:   &reviewerName,
		ModelName:      &modelName,
	})
	if err != nil {
		log.Printf("quoting-agent: failed to persist exhausted quote critic review: %v", err)
		return
	}
	reqDeps.SetLastQuoteReviewResult(reviewResult)
	q.emitQuoteCriticHumanAlert(ctx, reqDeps, leadID, serviceID, tenantID, summary)
}

func (q *QuotingAgent) persistQuoteCriticRepeatedFindings(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, previousSummary string) {
	draftResult := reqDeps.GetLastDraftResult()
	if draftResult == nil || reqDeps == nil || reqDeps.QuoteDrafter == nil {
		return
	}

	runID := reqDeps.GetRunID()
	reviewerName := "QuoteCritic"
	modelName := "moonshot"
	attemptCount := reqDeps.GetQuoteCriticAttempt()
	if attemptCount <= 0 {
		attemptCount = 2
	}

	summary := "AI-review meldde na herstel opnieuw dezelfde bevindingen; menselijke controle vereist."
	if strings.TrimSpace(previousSummary) != "" {
		summary = summary + " Laatste beoordeling: " + strings.TrimSpace(previousSummary)
	}

	reviewResult, err := reqDeps.QuoteDrafter.RecordQuoteAIReview(ctx, ports.RecordQuoteAIReviewParams{
		QuoteID:        draftResult.QuoteID,
		OrganizationID: tenantID,
		Decision:       ports.QuoteAIReviewDecisionRequiresHuman,
		Summary:        summary,
		Signals:        []string{"repair_findings_repeated"},
		AttemptCount:   attemptCount,
		RunID:          &runID,
		ReviewerName:   &reviewerName,
		ModelName:      &modelName,
	})
	if err != nil {
		log.Printf("quoting-agent: failed to persist repeated quote critic review: %v", err)
		return
	}

	reqDeps.SetLastQuoteReviewResult(reviewResult)
	q.emitQuoteCriticHumanAlert(ctx, reqDeps, leadID, serviceID, tenantID, summary)
}

func (q *QuotingAgent) emitQuoteCriticHumanAlert(ctx context.Context, reqDeps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, summary string) {
	if !reqDeps.MarkAlertEmitted("quote_critic_requires_human", "quote_critic_requires_human", summary) {
		return
	}
	_, _ = q.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      "QuoteCritic",
		EventType:      repository.EventTypeAlert,
		Title:          repository.EventTitleManualIntervention,
		Summary:        &summary,
		Metadata: repository.AlertMetadata{
			Trigger:   "quote_critic_requires_human",
			ErrorCode: "quote_critic_requires_human",
		}.ToMap(),
	})
	if _, err := q.repo.UpdatePipelineStage(ctx, serviceID, tenantID, domain.PipelineStageManualIntervention); err != nil {
		log.Printf("quoting-agent: failed to update stage to Manual_Intervention for service=%s: %v", serviceID, err)
	}
}

func (q *QuotingAgent) fetchEstimationGuidelines(ctx context.Context, tenantID uuid.UUID, serviceType string) string {
	return fetchServiceTypeEstimationGuidelines(ctx, q.repo, tenantID, serviceType)
}

func (q *QuotingAgent) runWithPrompt(ctx context.Context, promptText, userID string) error {
	return q.runWithPromptUsingTools(ctx, promptText, userID, "EstimatorRunner", "Runs the configured estimator agent", nil)
}

func (q *QuotingAgent) runWithPromptUsingTools(ctx context.Context, promptText, userID, agentName, description string, tools []tool.Tool) error {
	activeRunner := q.runner
	activeSessionService := q.sessionService
	activeAppName := q.appName

	if len(tools) > 0 {
		kimi := moonshot.NewModel(q.modelConfig)
		workspace, err := orchestration.LoadAgentWorkspace("calculator")
		if err != nil {
			return fmt.Errorf("failed to load calculator workspace context: %w", err)
		}
		instruction, err := orchestration.BuildAgentInstruction("calculator", "Follow prompt instructions and return tool calls only.")
		if err != nil {
			return fmt.Errorf("failed to load calculator workspace context: %w", err)
		}
		toolsets := orchestration.BuildWorkspaceToolsets(workspace, strings.ToLower(agentName)+"_tools", tools)
		dynamicAgent, err := llmagent.New(llmagent.Config{
			Name:        agentName,
			Model:       kimi,
			Description: description,
			Instruction: instruction,
			Toolsets:    toolsets,
		})
		if err != nil {
			return fmt.Errorf("failed to create dynamic quoting agent: %w", err)
		}

		activeSessionService = session.InMemoryService()
		activeAppName = strings.ToLower(agentName)
		activeRunner, err = runner.New(runner.Config{
			AppName:        activeAppName,
			Agent:          dynamicAgent,
			SessionService: activeSessionService,
		})
		if err != nil {
			return fmt.Errorf("failed to create dynamic runner: %w", err)
		}
	}

	sessionID := uuid.New().String()

	_, err := activeSessionService.Create(ctx, &session.CreateRequest{
		AppName:   activeAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create quoting session: %w", err)
	}
	defer func() {
		_ = activeSessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   activeAppName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var toolTrace []observedToolTrace
	err = consumeRunEvents(activeRunner.Run(ctx, userID, sessionID, userMessage, runConfig), "quoting agent run failed", func(event *session.Event) {
		_ = event
	}, observeSessionToolTrace(&toolTrace))
	logObservedToolTrace(strings.ToLower(agentName), userID, sessionID, toolTrace)
	if err != nil {
		return err
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
