package adapters

import (
	"context"
	"testing"
	"time"

	identitysvc "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

type stubWhatsAppReplyGenerator struct {
	lastInput ports.WhatsAppReplyInput
	result    ports.ReplySuggestionDraft
	err       error
}

func (s *stubWhatsAppReplyGenerator) SuggestWhatsAppReply(_ context.Context, input ports.WhatsAppReplyInput) (ports.ReplySuggestionDraft, error) {
	s.lastInput = input
	return s.result, s.err
}

func TestWhatsAppReplySuggesterAdapterMapsInput(t *testing.T) {
	now := time.Now().UTC()
	organizationID := uuid.New()
	requesterUserID := uuid.New()
	leadID := uuid.New()
	conversationID := uuid.New()
	generator := &stubWhatsAppReplyGenerator{result: ports.ReplySuggestionDraft{Text: "voorstel", EffectiveScenario: ports.ReplySuggestionScenarioAppointmentReminder}}
	adapter := NewWhatsAppReplySuggesterAdapter(generator)

	result, err := adapter.SuggestReply(context.Background(), identitysvc.SuggestWhatsAppReplyInput{
		OrganizationID:  organizationID,
		RequesterUserID: requesterUserID,
		LeadID:          &leadID,
		ConversationID:  conversationID,
		Scenario:        "appointment_reminder",
		ScenarioNotes:   "Noem ook dat de klant ons kan appen bij vragen.",
		PhoneNumber:     "+31612345678",
		DisplayName:     "Robin",
		Messages: []identitysvc.SuggestWhatsAppReplyMessage{{
			Direction: "inbound",
			Body:      "Kunnen jullie morgen bellen?",
			CreatedAt: now,
		}},
		Examples: []identitysvc.SuggestWhatsAppReplyExample{{
			CustomerMessage: "Vraag",
			Reply:           "Antwoord",
			CreatedAt:       now,
		}},
		Feedback: []identitysvc.SuggestWhatsAppReplyFeedback{{
			AIReply:    "AI tekst",
			HumanReply: "Mens tekst",
			CreatedAt:  now,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "voorstel" {
		t.Fatalf("expected mapped result, got %+v", result)
	}
	if result.EffectiveScenario != ports.ReplySuggestionScenarioAppointmentReminder {
		t.Fatalf("expected effective scenario to be preserved, got %+v", result)
	}
	assertMappedWhatsAppReplyInput(t, generator.lastInput, organizationID, requesterUserID, leadID, conversationID)
}

func assertMappedWhatsAppReplyInput(t *testing.T, input ports.WhatsAppReplyInput, organizationID, requesterUserID, leadID, conversationID uuid.UUID) {
	t.Helper()

	if input.OrganizationID != organizationID {
		t.Fatalf("expected organization id to be mapped")
	}
	if input.RequesterUserID != requesterUserID {
		t.Fatalf("expected requester user id to be mapped")
	}
	if input.LeadID == nil || *input.LeadID != leadID {
		t.Fatalf("expected lead id to be mapped")
	}
	if input.ConversationID != conversationID {
		t.Fatalf("expected conversation id to be mapped")
	}
	if input.Scenario != ports.ReplySuggestionScenarioAppointmentReminder {
		t.Fatalf("expected scenario to be normalized, got %q", input.Scenario)
	}
	if input.ScenarioNotes != "Noem ook dat de klant ons kan appen bij vragen." {
		t.Fatalf("expected scenario notes to be mapped, got %q", input.ScenarioNotes)
	}
	if len(input.Messages) != 1 || input.Messages[0].Body != "Kunnen jullie morgen bellen?" {
		t.Fatalf("expected messages to be mapped, got %+v", input.Messages)
	}
	if len(input.Examples) != 1 || input.Examples[0].Reply != "Antwoord" {
		t.Fatalf("expected examples to be mapped, got %+v", input.Examples)
	}
	if len(input.Feedback) != 1 || input.Feedback[0].HumanReply != "Mens tekst" {
		t.Fatalf("expected feedback to be mapped, got %+v", input.Feedback)
	}
}
