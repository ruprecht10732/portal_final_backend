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

// GenerateResult is the return value from a prompt-based quote generation.
type GenerateResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

// QuoteGenerator is a dedicated mini-agent that generates draft quotes from a
// user prompt. It has only SearchProductMaterials + DraftQuote tools, ensuring
// exact parity with the estimator's search→hydrate→draft pipeline without
// triggering SaveEstimation or UpdatePipelineStage.
type QuoteGenerator struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	runMu          sync.Mutex
}

// QuoteGeneratorConfig holds configuration for creating a QuoteGenerator.
type QuoteGeneratorConfig struct {
	APIKey              string
	Repo                repository.LeadsRepository
	EventBus            events.Bus
	EmbeddingClient     *embeddings.Client
	QdrantClient        *qdrant.Client
	CatalogQdrantClient *qdrant.Client
	CatalogReader       ports.CatalogReader
	QuoteDrafter        ports.QuoteDrafter
}

// NewQuoteGenerator creates a QuoteGenerator agent with only product search and
// quote drafting capabilities.
func NewQuoteGenerator(cfg QuoteGeneratorConfig) (*QuoteGenerator, error) {
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

	var tools []tool.Tool

	// Calculator is always available for exact arithmetic.
	calculatorTool, err := createCalculatorTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build Calculator tool: %w", err)
	}
	tools = append(tools, calculatorTool)

	// DraftQuote is always registered (gracefully errors if drafter is nil).
	draftQuoteTool, err := createDraftQuoteTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build DraftQuote tool: %w", err)
	}
	tools = append(tools, draftQuoteTool)

	// Add product search tool if configured.
	if deps.IsProductSearchEnabled() {
		searchTool, err := createSearchProductMaterialsTool(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build SearchProductMaterials tool: %w", err)
		}
		tools = append(tools, searchTool)
		log.Printf("QuoteGenerator: product search enabled")
	} else {
		log.Printf("QuoteGenerator: product search disabled")
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "QuoteGenerator",
		Model:       kimi,
		Description: "Generates draft quotes from a user prompt using catalog search.",
		Instruction: "You are a Quote Generator. Search for products and create draft quotes.",
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create quote generator agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "quote-generator",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create quote generator runner: %w", err)
	}

	return &QuoteGenerator{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "quote-generator",
		repo:           cfg.Repo,
		toolDeps:       deps,
	}, nil
}

// SetCatalogReader injects the catalog reader (set after construction to break circular deps).
func (g *QuoteGenerator) SetCatalogReader(cr ports.CatalogReader) {
	g.toolDeps.CatalogReader = cr
}

// SetQuoteDrafter injects the quote drafter (set after construction to break circular deps).
func (g *QuoteGenerator) SetQuoteDrafter(qd ports.QuoteDrafter) {
	g.toolDeps.QuoteDrafter = qd
}

// Generate runs the quote generator agent with the user's prompt and lead context.
// If existingQuoteID is non-nil, the DraftQuote tool will update the existing quote.
// It returns the generated draft quote result or an error.
func (g *QuoteGenerator) Generate(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userPrompt string, existingQuoteID *uuid.UUID) (*GenerateResult, error) {
	g.runMu.Lock()
	defer g.runMu.Unlock()

	g.toolDeps.SetTenantID(tenantID)
	g.toolDeps.SetLeadContext(leadID, serviceID)
	g.toolDeps.SetActor("AI", "Quote Generator")
	g.toolDeps.ResetToolCallTracking()
	g.toolDeps.SetExistingQuoteID(existingQuoteID)

	lead, err := g.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quote generator: load lead: %w", err)
	}
	service, err := g.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quote generator: load service: %w", err)
	}

	notes, err := g.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		log.Printf("QuoteGenerator: notes fetch failed: %v", err)
		notes = nil
	}

	promptText := buildQuoteGeneratePrompt(lead, service, notes, userPrompt)

	sessionID := uuid.New().String()
	userID := "quote-gen-" + leadID.String()

	_, err = g.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   g.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("quote generator: create session: %w", err)
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

	result := g.toolDeps.GetLastDraftResult()
	if result == nil {
		return nil, fmt.Errorf("quote generator: agent did not produce a draft quote")
	}

	return &GenerateResult{
		QuoteID:     result.QuoteID,
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}
