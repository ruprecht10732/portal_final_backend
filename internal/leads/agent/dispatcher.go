package agent

import (
	"context"
	"fmt"
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
	apptools "portal_final_backend/internal/tools"
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
}

// NewDispatcher creates a Dispatcher agent.
func NewDispatcher(apiKey string, modelName string, repo repository.LeadsRepository, eventBus events.Bus) (*Dispatcher, error) {
	kimi := moonshot.NewModel(newMoonshotReasoningModelConfig(apiKey, modelName))
	workspace, err := orchestration.LoadAgentWorkspace("matchmaker")
	if err != nil {
		return nil, fmt.Errorf("failed to load matchmaker workspace context: %w", err)
	}

	deps := &ToolDependencies{
		Repo:           repo,
		EventBus:       eventBus,
		CouncilService: NewDefaultMultiAgentCouncil(repo),
	}

	findPartnersTool, err := apptools.NewFindMatchingPartnersTool(func(ctx tool.Context, input FindMatchingPartnersInput) (FindMatchingPartnersOutput, error) {
		return handleFindMatchingPartners(ctx, GetDependencies(ctx), input)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build FindMatchingPartners tool: %w", err)
	}

	updateStageTool, err := apptools.NewUpdatePipelineStageTool(func(ctx tool.Context, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
		return handleUpdatePipelineStage(ctx, GetDependencies(ctx), input)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdatePipelineStage tool: %w", err)
	}

	createOfferTool, err := apptools.NewCreatePartnerOfferTool(func(ctx tool.Context, input CreatePartnerOfferInput) (CreatePartnerOfferOutput, error) {
		deps := GetDependencies(ctx)
		if deps.OfferCreator == nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Offer creation not configured"}, fmt.Errorf("offer creator not configured")
		}

		tenantID, serviceID, partnerID, hours, contextMessage, err := resolveOfferContext(deps, input.PartnerID, input.ExpirationHours)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: contextMessage}, err
		}

		quoteID, err := deps.Repo.GetLatestAcceptedQuoteIDForService(ctx, serviceID, tenantID)
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: "Accepted quote not found for service"}, err
		}

		summary := truncateRunes(strings.TrimSpace(input.JobSummaryShort), 200)
		result, err := deps.OfferCreator.CreateOfferFromQuote(ctx, tenantID, ports.CreateOfferFromQuoteParams{
			PartnerID:       partnerID,
			QuoteID:         quoteID,
			ExpiresInHours:  hours,
			JobSummaryShort: summary,
		})
		if err != nil {
			return CreatePartnerOfferOutput{Success: false, Message: err.Error()}, err
		}

		deps.MarkOfferCreated()
		return CreatePartnerOfferOutput{Success: true, Message: "Offer created", OfferID: result.OfferID.String(), PublicToken: result.PublicToken}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build CreatePartnerOffer tool: %w", err)
	}
	toolsets := orchestration.BuildWorkspaceToolsets(workspace, "matchmaker_tools", []tool.Tool{findPartnersTool, createOfferTool, updateStageTool})

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "Dispatcher",
		Model:       kimi,
		Description: "Fulfillment manager that finds partner matches and advances the pipeline.",
		Instruction: workspace.Instruction,
		Toolsets:    toolsets,
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

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader.
func (d *Dispatcher) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	if d == nil || d.toolDeps == nil {
		return
	}
	d.toolDeps.SetOrganizationAISettingsReader(reader)
}

// SetOfferCreator injects the partner offer creator after module initialization.
func (d *Dispatcher) SetOfferCreator(creator ports.PartnerOfferCreator) {
	d.toolDeps.mu.Lock()
	defer d.toolDeps.mu.Unlock()
	d.toolDeps.OfferCreator = creator
}

// Run executes partner matching for a lead service.
func (d *Dispatcher) Run(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	reqDeps := d.toolDeps.NewRequestDeps()
	reqDeps.SetTenantID(tenantID)
	reqDeps.SetLeadContext(leadID, serviceID)
	reqDeps.SetActor(repository.ActorTypeAI, repository.ActorNameDispatcher)
	reqDeps.ResetToolCallTracking()
	runID := reqDeps.GetRunID()
	fmt.Printf("dispatcher: run started runID=%s lead=%s service=%s tenant=%s\n", runID, leadID, serviceID, tenantID)

	ctx = WithDependencies(ctx, reqDeps)

	// Preload org settings for consistency across agents (even if not used directly today).
	if _, err := reqDeps.LoadOrganizationAISettings(ctx); err != nil {
		fmt.Printf("dispatcher: failed to load org AI settings (tenant=%s): %v\n", tenantID, err)
	}

	lead, err := d.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := d.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	aggs, err := d.repo.GetServiceStateAggregates(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}
	if d.shouldSkipDispatch(ctx, leadID, tenantID, aggs) {
		return nil
	}

	excludedIDs, err := d.repo.GetInvitedPartnerIDs(ctx, serviceID)
	if err != nil {
		fmt.Printf("Dispatcher warning: failed to fetch exclusions: %v\n", err)
		excludedIDs = []uuid.UUID{}
	}

	promptText := buildDispatcherPrompt(lead, service, 25, excludedIDs)
	if err := d.runWithPrompt(ctx, promptText, leadID); err != nil {
		return err
	}

	d.ensureDispatchPostconditions(ctx, runID, leadID, serviceID, tenantID, service.PipelineStage)
	fmt.Printf("dispatcher: run finished runID=%s lead=%s service=%s\n", runID, leadID, serviceID)

	return nil
}

func (d *Dispatcher) shouldSkipDispatch(ctx context.Context, leadID, tenantID uuid.UUID, aggs repository.ServiceStateAggregates) bool {
	if aggs.AcceptedOffers > 0 || aggs.PendingOffers > 0 {
		// A partner-offer flow is already in progress (or accepted); do not re-dispatch.
		return true
	}
	if linked, err := d.repo.HasLinkedPartners(ctx, tenantID, leadID); err == nil && linked {
		// A human linked at least one partner to this lead; do not override with AI dispatch.
		return true
	}
	return false
}

func (d *Dispatcher) ensureDispatchPostconditions(ctx context.Context, runID string, leadID, serviceID, tenantID uuid.UUID, stageAtStart string) {
	reqDeps := GetDependencies(ctx)
	// Re-read the current stage. If it changed since the run started a human
	// (or another process) already acted — do not override their decision.
	if current, err := d.repo.GetLeadServiceByID(ctx, serviceID, tenantID); err == nil {
		if current.PipelineStage != stageAtStart {
			fmt.Printf("Dispatcher: stage changed externally during run (was %s, now %s), skipping fallback runID=%s\n", stageAtStart, current.PipelineStage, runID)
			return
		}
	}

	if !reqDeps.WasStageUpdateCalled() {
		if _, err := d.repo.UpdatePipelineStage(ctx, serviceID, tenantID, domain.PipelineStageManualIntervention); err != nil {
			fmt.Printf("Dispatcher warning: fallback stage update failed runID=%s lead=%s service=%s err=%v\n", runID, leadID, serviceID, err)
		} else {
			fmt.Printf("Dispatcher warning: no stage update recorded, fallback to Manual_Intervention runID=%s lead=%s service=%s\n", runID, leadID, serviceID)
		}
	}
	if reqDeps.LastStageUpdated() == domain.PipelineStageFulfillment && !reqDeps.WasOfferCreated() {
		fmt.Printf("Dispatcher warning: Fulfillment without offer runID=%s lead=%s service=%s\n", runID, leadID, serviceID)
	}
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
	var toolTrace []observedToolTrace
	err = consumeRunEvents(d.runner.Run(ctx, userID, sessionID, userMessage, runConfig), "dispatcher run failed", func(event *session.Event) {
		_ = event
	}, observeSessionToolTrace(&toolTrace))
	logObservedToolTrace("dispatcher", userID, sessionID, toolTrace)
	if err != nil {
		return err
	}

	return nil
}
