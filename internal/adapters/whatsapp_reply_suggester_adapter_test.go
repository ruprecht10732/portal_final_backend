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
	result    string
	err       error
}

func (s *stubWhatsAppReplyGenerator) SuggestWhatsAppReply(_ context.Context, input ports.WhatsAppReplyInput) (string, error) {
	s.lastInput = input
	return s.result, s.err
}

func TestWhatsAppReplySuggesterAdapterMapsInput(t *testing.T) {
	now := time.Now().UTC()
	organizationID := uuid.New()
	requesterUserID := uuid.New()
	leadID := uuid.New()
	conversationID := uuid.New()
	generator := &stubWhatsAppReplyGenerator{result: "voorstel"}
	adapter := NewWhatsAppReplySuggesterAdapter(generator)

	result, err := adapter.SuggestReply(context.Background(), identitysvc.SuggestWhatsAppReplyInput{
		OrganizationID:  organizationID,
		RequesterUserID: requesterUserID,
		LeadID:          leadID,
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
	if result != "voorstel" {
		t.Fatalf("expected mapped result, got %q", result)
	}
	if generator.lastInput.OrganizationID != organizationID {
		t.Fatalf("expected organization id to be mapped")
	}
	if generator.lastInput.RequesterUserID != requesterUserID {
		t.Fatalf("expected requester user id to be mapped")
	}
	if generator.lastInput.LeadID != leadID {
		t.Fatalf("expected lead id to be mapped")
	}
	if generator.lastInput.ConversationID != conversationID {
		t.Fatalf("expected conversation id to be mapped")
	}
	if generator.lastInput.Scenario != ports.ReplySuggestionScenarioAppointmentReminder {
		t.Fatalf("expected scenario to be normalized, got %q", generator.lastInput.Scenario)
	}
	if generator.lastInput.ScenarioNotes != "Noem ook dat de klant ons kan appen bij vragen." {
		t.Fatalf("expected scenario notes to be mapped, got %q", generator.lastInput.ScenarioNotes)
	}
	if len(generator.lastInput.Messages) != 1 || generator.lastInput.Messages[0].Body != "Kunnen jullie morgen bellen?" {
		t.Fatalf("expected messages to be mapped, got %+v", generator.lastInput.Messages)
	}
	if len(generator.lastInput.Examples) != 1 || generator.lastInput.Examples[0].Reply != "Antwoord" {
		t.Fatalf("expected examples to be mapped, got %+v", generator.lastInput.Examples)
	}
	if len(generator.lastInput.Feedback) != 1 || generator.lastInput.Feedback[0].HumanReply != "Mens tekst" {
		t.Fatalf("expected feedback to be mapped, got %+v", generator.lastInput.Feedback)
	}
}
