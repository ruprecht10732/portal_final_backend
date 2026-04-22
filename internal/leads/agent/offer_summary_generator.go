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
	"portal_final_backend/platform/ai/openaicompat"
)

// OfferSummaryGenerator produces short, readable summaries for partner offers.
type OfferSummaryGenerator struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
}

// NewOfferSummaryGenerator creates a summary generator agent without tools.
func NewOfferSummaryGenerator(modelCfg openaicompat.Config, sessionService session.Service) (*OfferSummaryGenerator, error) {
	kimi := openaicompat.NewModel(modelCfg)
	workspace, err := orchestration.LoadAgentWorkspace("offer-summary")
	if err != nil {
		return nil, fmt.Errorf("failed to load offer summary workspace context: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "OfferSummaryGenerator",
		Model:       kimi,
		Description: "Generates concise, readable summaries for partner offers.",
		Instruction: workspace.Instruction,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create offer summary agent: %w", err)
	}

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

// GenerateOfferSummary renders a readable summary using only allowed fields.
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
Write TWO summaries for a professional contractor.

PART 1 – Short summary (one plain-text line, max 150 characters, no markdown):
A compact description of the job a vakman can scan at a glance.
Example: "Dakreparatie: 2 dakpannen vervangen, goot herstellen"

Then write a line containing exactly: ---

PART 2 – Detailed summary (plain text, max 8 lines):
Rules:
- Output only plain text, no extra commentary.
- Use Dutch.
- Write as text that is shown directly to a vakman deciding whether to accept the job.
- Use natural, readable Dutch. Prefer concrete work language over generic wording.
- The first sentence must read like a human-written klusomschrijving, not like a label list or internal note.
- Do NOT include any personal data: no names, addresses, phone numbers, emails.
- Only use the provided service type, scope, urgency, and line items.
- If scope or urgency is missing, omit that label.
- Do not use markdown syntax such as ###, **, -, * or numbered lists.
- Preferred structure:
	1) Optional line like: Omvang: Groot | Urgentie: Hoog.
	2) One short sentence that explains in plain Dutch what kind of klus this is and what the vakman mainly doet.
	3) A line containing exactly: Werkzaamheden
	4) Up to 3 short lines with the main werkzaamheden or inbegrepen onderdelen.
	5) Optional line containing exactly: Let op
	6) Up to 2 short lines with concrete aandachtspunten for inspectie, planning, bereikbaarheid or omvang.
- Avoid vague filler like "werkzaamheden uitvoeren" when the line items are more specific.
- Keep the tone practical and easy to scan on mobile.
`, strings.TrimSpace(input.ServiceType), scope, urgency, strings.Join(lines, "\n"))
}
