package agent

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

type FirstResponder struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	appName        string
	repo           repository.LeadsRepository
}

// NewFirstResponder builds the ADK agent with Kimi + Tools
func NewFirstResponder(apiKey string, repo repository.LeadsRepository) *FirstResponder {
	kimi := moonshot.NewModel(moonshot.Config{APIKey: apiKey})

	type markUrgentInput struct {
		LeadID string `json:"leadId"`
		Reason string `json:"reason"`
	}
	type markUrgentOutput struct {
		Message string `json:"message"`
	}
	markUrgentTool, err := functiontool.New(functiontool.Config{
		Name:        "MarkAsUrgent",
		Description: "Flags a lead as urgent",
	}, func(ctx tool.Context, input markUrgentInput) (markUrgentOutput, error) {
		id, _ := uuid.Parse(input.LeadID)
		lead, err := repo.GetByID(context.Background(), id)
		if err != nil {
			return markUrgentOutput{}, err
		}
		if lead.AssignedAgentID == nil {
			return markUrgentOutput{}, fmt.Errorf("lead has no assigned agent")
		}
		_, err = repo.CreateLeadNote(context.Background(), repository.CreateLeadNoteParams{
			LeadID:   id,
			AuthorID: *lead.AssignedAgentID,
			Type:     "system",
			Body:     "URGENT FLAG: " + input.Reason,
		})
		if err != nil {
			return markUrgentOutput{}, err
		}
		return markUrgentOutput{Message: "Lead marked as urgent"}, nil
	})
	if err != nil {
		log.Printf("failed to create MarkAsUrgent tool: %v", err)
	}

	type draftEmailInput struct {
		LeadID      string `json:"leadId"`
		MissingInfo string `json:"missingInfo"`
	}
	type draftEmailOutput struct {
		Message string `json:"message"`
	}
	draftEmailTool, err := functiontool.New(functiontool.Config{
		Name:        "DraftMissingInfoEmail",
		Description: "Drafts an email for missing info",
	}, func(ctx tool.Context, input draftEmailInput) (draftEmailOutput, error) {
		id, _ := uuid.Parse(input.LeadID)
		lead, err := repo.GetByID(context.Background(), id)
		if err != nil {
			return draftEmailOutput{}, err
		}
		if lead.AssignedAgentID == nil {
			return draftEmailOutput{}, fmt.Errorf("lead has no assigned agent")
		}
		body := fmt.Sprintf("DRAFT: Hi, we need the following info to proceed: %s", input.MissingInfo)
		_, err = repo.CreateLeadNote(context.Background(), repository.CreateLeadNoteParams{
			LeadID:   id,
			AuthorID: *lead.AssignedAgentID,
			Type:     "email",
			Body:     body,
		})
		if err != nil {
			return draftEmailOutput{}, err
		}
		return draftEmailOutput{Message: "Draft created"}, nil
	})
	if err != nil {
		log.Printf("failed to create DraftMissingInfoEmail tool: %v", err)
	}

	tools := make([]tool.Tool, 0, 2)
	if markUrgentTool != nil {
		tools = append(tools, markUrgentTool)
	}
	if draftEmailTool != nil {
		tools = append(tools, draftEmailTool)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "LeadFirstResponder",
		Model:       kimi,
		Description: "Active first responder agent for incoming leads.",
		Instruction: `You are an Active First Responder Sales Agent.
Analyze the incoming lead data carefully.

PROTOCOL:
1. If the lead mentions an emergency (e.g., "leak", "broken", "danger"), use the 'MarkAsUrgent' tool immediately.
2. If the lead is missing critical info (Phone or Email), use the 'DraftMissingInfoEmail' tool.
3. Otherwise, simply output a brief summary of the lead quality (High/Medium/Low).`,
		Tools: tools,
	})
	if err != nil {
		log.Printf("failed to create ADK agent: %v", err)
	}

	appName := "lead_first_responder"
	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		log.Printf("failed to create ADK runner: %v", err)
	}

	return &FirstResponder{
		agent:          adkAgent,
		runner:         r,
		sessionService: sessionService,
		appName:        appName,
		repo:           repo,
	}
}

// Process runs the agent on a specific lead
func (fr *FirstResponder) Process(ctx context.Context, leadID uuid.UUID) error {
	if fr.runner == nil {
		return fmt.Errorf("first responder runner is not initialized")
	}
	if fr.sessionService == nil {
		return fmt.Errorf("first responder session service is not initialized")
	}
	lead, err := fr.repo.GetByID(ctx, leadID)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf(`
Lead ID: %s
Name: %s %s
Role: %s
Service: %s
Notes: %s
Phone: %s
Email: %v
`, lead.ID, lead.ConsumerFirstName, lead.ConsumerLastName,
		lead.ConsumerRole, lead.ServiceType, getValue(lead.ConsumerNote),
		lead.ConsumerPhone, getValue(lead.ConsumerEmail))

	userMessage := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}

	var output string
	userID := "lead-" + leadID.String()
	sessionID := uuid.New().String() // Fresh session for each invocation

	_, err = fr.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   fr.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	log.Printf("Created session %s for lead %s", sessionID, leadID)

	for event, err := range fr.runner.Run(ctx, userID, sessionID, userMessage, agent.RunConfig{StreamingMode: agent.StreamingModeNone}) {
		if err != nil {
			return err
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				output += part.Text
			}
		}
	}

	log.Printf("Agent finished processing lead %s. Output: %s", leadID, output)
	return nil
}

func getValue(s *string) string {
	if s == nil {
		return "N/A"
	}
	return *s
}
