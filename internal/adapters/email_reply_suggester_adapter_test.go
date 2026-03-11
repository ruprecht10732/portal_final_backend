package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	imapsvc "portal_final_backend/internal/imap/service"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

type stubEmailReplyGenerator struct {
	lastInput ports.EmailReplyInput
	result    ports.ReplySuggestionDraft
	err       error
}

func (s *stubEmailReplyGenerator) SuggestEmailReply(_ context.Context, input ports.EmailReplyInput) (ports.ReplySuggestionDraft, error) {
	s.lastInput = input
	return s.result, s.err
}

func TestEmailReplySuggesterAdapterMapsInput(t *testing.T) {
	now := time.Now().UTC()
	organizationID := uuid.New()
	requesterUserID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	generator := &stubEmailReplyGenerator{result: ports.ReplySuggestionDraft{Text: "voorstel", EffectiveScenario: ports.ReplySuggestionScenarioQuoteReminder}}
	adapter := NewEmailReplySuggesterAdapter(generator)

	result, err := adapter.SuggestReply(context.Background(), imapsvc.SuggestEmailReplyInput{
		OrganizationID:  organizationID,
		RequesterUserID: requesterUserID,
		AccountID:       uuid.New(),
		MessageUID:      12,
		LeadID:          &leadID,
		LeadServiceID:   &serviceID,
		Scenario:        "quote_reminder",
		ScenarioNotes:   "Benadruk dat we beschikbaar zijn voor vragen.",
		CustomerEmail:   "customer@example.com",
		CustomerName:    "Robin",
		Subject:         "Planning",
		MessageBody:     "Kunnen jullie vrijdag?",
		Feedback: []imapsvc.SuggestEmailReplyFeedback{{
			AIReply:    "AI tekst",
			HumanReply: "Mens tekst",
			CreatedAt:  now,
		}},
		Examples: []imapsvc.SuggestEmailReplyExample{{
			CustomerMessage: "Vraag",
			Reply:           "Antwoord",
			CreatedAt:       now,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "voorstel" {
		t.Fatalf("expected mapped result, got %+v", result)
	}
	if result.EffectiveScenario != ports.ReplySuggestionScenarioQuoteReminder {
		t.Fatalf("expected effective scenario to be preserved, got %+v", result)
	}
	if generator.lastInput.OrganizationID != organizationID {
		t.Fatalf("expected organization id to be mapped")
	}
	if generator.lastInput.RequesterUserID != requesterUserID {
		t.Fatalf("expected requester user id to be mapped")
	}
	if generator.lastInput.LeadID == nil || *generator.lastInput.LeadID != leadID {
		t.Fatalf("expected lead id to be mapped, got %+v", generator.lastInput.LeadID)
	}
	if generator.lastInput.LeadServiceID == nil || *generator.lastInput.LeadServiceID != serviceID {
		t.Fatalf("expected lead service id to be mapped, got %+v", generator.lastInput.LeadServiceID)
	}
	if generator.lastInput.Scenario != ports.ReplySuggestionScenarioQuoteReminder {
		t.Fatalf("expected scenario to be normalized, got %q", generator.lastInput.Scenario)
	}
	if generator.lastInput.ScenarioNotes != "Benadruk dat we beschikbaar zijn voor vragen." {
		t.Fatalf("expected scenario notes to be mapped, got %q", generator.lastInput.ScenarioNotes)
	}
	if len(generator.lastInput.Feedback) != 1 || generator.lastInput.Feedback[0].AIReply != "AI tekst" {
		t.Fatalf("expected feedback to be mapped, got %+v", generator.lastInput.Feedback)
	}
	if len(generator.lastInput.Examples) != 1 || generator.lastInput.Examples[0].Reply != "Antwoord" {
		t.Fatalf("expected examples to be mapped, got %+v", generator.lastInput.Examples)
	}
}

func TestEmailReplySuggesterAdapterMapsLeadContextError(t *testing.T) {
	adapter := NewEmailReplySuggesterAdapter(&stubEmailReplyGenerator{err: ports.ErrEmailReplyLeadContextUnavailable})

	_, err := adapter.SuggestReply(context.Background(), imapsvc.SuggestEmailReplyInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if errors.Is(err, ports.ErrEmailReplyLeadContextUnavailable) {
		t.Fatal("expected domain error to be translated")
	}
}
