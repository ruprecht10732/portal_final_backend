package agent

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"portal_final_backend/internal/events"
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
	APIKey              string
	Repo                repository.LeadsRepository
	EventBus            events.Bus
	EmbeddingClient     *embeddings.Client  // Optional: enables product search
	QdrantClient        *qdrant.Client      // Optional: fallback collection search
	CatalogQdrantClient *qdrant.Client      // Optional: catalog collection search
	CatalogReader       ports.CatalogReader // Optional: hydrate search results from DB
	QuoteDrafter        ports.QuoteDrafter  // Optional: draft quotes from agent
}

// NewEstimator creates an Estimator agent.
func NewEstimator(cfg EstimatorConfig) (*Estimator, error) {
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          cfg.APIKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &ToolDependencies{
		Repo:                cfg.Repo,
		EventBus:            cfg.EventBus,
		EmbeddingClient:     cfg.EmbeddingClient,
		QdrantClient:        cfg.QdrantClient,
		CatalogQdrantClient: cfg.CatalogQdrantClient,
		CatalogReader:       cfg.CatalogReader,
		QuoteDrafter:        cfg.QuoteDrafter,
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
	e.toolDeps.SetActor("AI", "Estimator")

	lead, err := e.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := e.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	notes, err := e.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		log.Printf("estimator notes fetch failed: %v", err)
		notes = nil
	}

	var photo *repository.PhotoAnalysis
	if analysis, err := e.repo.GetLatestPhotoAnalysis(ctx, serviceID, tenantID); err == nil {
		photo = &analysis
	}

	promptText := buildEstimatorPrompt(lead, service, notes, photo)
	return e.runWithPrompt(ctx, promptText, leadID)
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
