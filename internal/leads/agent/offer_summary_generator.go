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

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/orchestration"
	"portal_final_backend/platform/ai/moonshot"
)

// OfferSummaryGenerator produces short, markdown summaries for partner offers.
type OfferSummaryGenerator struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
}

// NewOfferSummaryGenerator creates a summary generator agent without tools.
func NewOfferSummaryGenerator(apiKey string, modelName string) (*OfferSummaryGenerator, error) {
	kimi := moonshot.NewModel(newMoonshotModelConfig(apiKey, modelName))
	workspace, err := orchestration.LoadAgentWorkspace("offer-summary")
	if err != nil {
		return nil, fmt.Errorf("failed to load offer summary workspace context: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "OfferSummaryGenerator",
		Model:       kimi,
		Description: "Generates concise, markdown summaries for partner offers.",
		Instruction: workspace.Instruction,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create offer summary agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "offer-summary-generator",
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create offer summary runner: %w", err)
	}

	return &OfferSummaryGenerator{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        "offer-summary-generator",
	}, nil
}

// GenerateOfferSummary renders a markdown summary using only allowed fields.
func (g *OfferSummaryGenerator) GenerateOfferSummary(ctx context.Context, tenantID uuid.UUID, input ports.OfferSummaryInput) (string, error) {
	_ = tenantID

	promptText := buildOfferSummaryPrompt(input)
	sessionID := uuid.New().String()
	userID := "offer-summary-" + input.LeadServiceID.String()
	outputText, err := runPromptTextSession(ctx, promptRunRequest{
		SessionService:       g.sessionService,
		Runner:               g.runner,
		AppName:              g.appName,
		UserID:               userID,
		SessionID:            sessionID,
		CreateSessionMessage: "offer summary: create session",
		RunFailureMessage:    "offer summary: run failed",
		TraceLabel:           "offer-summary",
	}, promptText)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(outputText), nil
}

func buildOfferSummaryPrompt(input ports.OfferSummaryInput) string {
	lines := make([]string, 0, len(input.Items))
	for _, item := range input.Items {
		label := strings.TrimSpace(item.Description)
		qty := strings.TrimSpace(item.Quantity)
		if label == "" {
			continue
		}
		if qty != "" {
			label = qty + " " + label
		}
		lines = append(lines, "- "+label)
	}

	scope := ""
	if input.Scope != nil {
		scope = strings.TrimSpace(*input.Scope)
	}
	urgency := ""
	if input.UrgencyLevel != nil {
		urgency = strings.TrimSpace(*input.UrgencyLevel)
	}

	return fmt.Sprintf(`Context:
- Service type: %s
- Scope: %s
- Urgency: %s

Line items:
%s

Task:
Write a short summary for a professional contractor.
Rules:
- Output only markdown, no extra commentary.
- Use Dutch.
- Do NOT include any personal data: no names, addresses, phone numbers, emails.
- Only use the provided service type, scope, urgency, and line items.
- If scope or urgency is missing, omit that label.
- Keep it concise (max 5 lines).
- Preferred structure:
	1) Optional line with **Omvang** and **Urgentie**.
	2) One short sentence describing the job.
	3) Numbered list of up to 3 main items.
`, strings.TrimSpace(input.ServiceType), scope, urgency, strings.Join(lines, "\n"))
}
