package agent

import (
	"context"
	"fmt"
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
	"portal_final_backend/platform/ai/moonshot"
)

// Dispatcher finds partner matches and advances pipeline stage.
type Dispatcher struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
	toolDeps       *ToolDependencies
	runMu          sync.Mutex
}

// NewDispatcher creates a Dispatcher agent.
func NewDispatcher(apiKey string, repo repository.LeadsRepository, eventBus events.Bus) (*Dispatcher, error) {
	kimi := moonshot.NewModel(moonshot.Config{
		APIKey:          apiKey,
		Model:           "kimi-k2.5",
		DisableThinking: true,
	})

	deps := &ToolDependencies{
		Repo:     repo,
		EventBus: eventBus,
	}

	findPartnersTool, err := createFindMatchingPartnersTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build FindMatchingPartners tool: %w", err)
	}

	updateStageTool, err := createUpdatePipelineStageTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdatePipelineStage tool: %w", err)
	}

	createOfferTool, err := createCreatePartnerOfferTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build CreatePartnerOffer tool: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "Dispatcher",
		Model:       kimi,
		Description: "Fulfillment manager that finds partner matches and advances the pipeline.",
		Instruction: "You are the Fulfillment Manager.",
		Tools:       []tool.Tool{findPartnersTool, createOfferTool, updateStageTool},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create dispatcher agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "dispatcher",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create dispatcher runner: %w", err)
	}

	return &Dispatcher{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "dispatcher",
		repo:           repo,
		toolDeps:       deps,
	}, nil
}

// SetOfferCreator injects the partner offer creator after module initialization.
func (d *Dispatcher) SetOfferCreator(creator ports.PartnerOfferCreator) {
	d.toolDeps.mu.Lock()
	defer d.toolDeps.mu.Unlock()
	d.toolDeps.OfferCreator = creator
}

// Run executes partner matching for a lead service.
func (d *Dispatcher) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	d.runMu.Lock()
	defer d.runMu.Unlock()

	d.toolDeps.SetTenantID(tenantID)
	d.toolDeps.SetLeadContext(leadID, serviceID)
	d.toolDeps.SetActor("AI", "Dispatcher")

	lead, err := d.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := d.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	promptText := buildDispatcherPrompt(lead, service, 25)
	return d.runWithPrompt(ctx, promptText, leadID)
}

func (d *Dispatcher) runWithPrompt(ctx context.Context, promptText string, leadID uuid.UUID) error {
	sessionID := uuid.New().String()
	userID := "dispatcher-" + leadID.String()

	_, err := d.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   d.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create dispatcher session: %w", err)
	}
	defer func() {
		_ = d.sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   d.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	for event := range d.runner.Run(ctx, userID, sessionID, userMessage, runConfig) {
		_ = event
	}

	return nil
}
