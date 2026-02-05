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
	updateServiceTypeTool, err := createUpdateLeadServiceTypeTool(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build UpdateLeadServiceType tool: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "Gatekeeper",
		Model:       kimi,
		Description: "Validates intake requirements and advances the lead pipeline.",
		Instruction: "You validate intake requirements and advance the pipeline stage.",
		Tools:       []tool.Tool{updateServiceTypeTool, updateStageTool},
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

	lead, err := g.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return err
	}
	service, err := g.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return err
	}

	notes, err := g.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		log.Printf("gatekeeper notes fetch failed: %v", err)
		notes = nil
	}

	attachments, err := g.repo.ListAttachmentsByService(ctx, serviceID, tenantID)
	if err != nil {
		log.Printf("gatekeeper attachments fetch failed: %v", err)
		attachments = nil
	}

	intakeContext := g.buildServiceContext(ctx, tenantID)
	promptText := buildGatekeeperPrompt(lead, service, notes, intakeContext, attachments)

	return g.runWithPrompt(ctx, promptText, leadID)
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
