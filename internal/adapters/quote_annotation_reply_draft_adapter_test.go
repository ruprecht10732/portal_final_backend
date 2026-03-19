package adapters

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/internal/leads/ports"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

func TestQuoteAnnotationReplyDraftAdapterMapsInput(t *testing.T) {
	now := time.Now().UTC()
	organizationID := uuid.New()
	requesterUserID := uuid.New()
	quoteID := uuid.New()
	leadID := uuid.New()
	generator := &stubWhatsAppReplyGenerator{result: ports.ReplySuggestionDraft{Text: " Voorstelantwoord "}}
	adapter := NewQuoteAnnotationReplyDraftAdapter(generator)

	result, err := adapter.SuggestReplyDraft(context.Background(), quotesvc.SuggestQuoteAnnotationReplyDraftInput{
		OrganizationID:  organizationID,
		RequesterUserID: requesterUserID,
		QuoteID:         quoteID,
		LeadID:          leadID,
		QuoteNumber:     "OFF-2026-0042",
		CustomerName:    "Robin",
		ItemTitle:       "Zonnepanelen",
		Messages: []quotesvc.QuoteAnnotationReplyDraftMessage{
			{AuthorType: "customer", Text: "Wat is de levertijd?", CreatedAt: now},
			{AuthorType: "agent", Text: "We stemmen de planning af.", CreatedAt: now.Add(time.Minute)},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Voorstelantwoord" {
		t.Fatalf("expected trimmed result text, got %+v", result)
	}
	if generator.lastInput.OrganizationID != organizationID {
		t.Fatalf("expected organization id to be mapped")
	}
	if generator.lastInput.RequesterUserID != requesterUserID {
		t.Fatalf("expected requester user id to be mapped")
	}
	if generator.lastInput.LeadID == nil || *generator.lastInput.LeadID != leadID {
		t.Fatalf("expected lead id to be mapped")
	}
	if generator.lastInput.ConversationID != quoteID {
		t.Fatalf("expected quote id to be reused as conversation id")
	}
	if generator.lastInput.DisplayName != "Robin" {
		t.Fatalf("expected customer name to be mapped, got %q", generator.lastInput.DisplayName)
	}
	if len(generator.lastInput.Messages) != 2 {
		t.Fatalf("expected messages to be mapped, got %+v", generator.lastInput.Messages)
	}
	if generator.lastInput.Messages[0].Direction != "inbound" {
		t.Fatalf("expected customer message to map to inbound, got %+v", generator.lastInput.Messages[0])
	}
	if generator.lastInput.Messages[1].Direction != "outbound" {
		t.Fatalf("expected agent message to map to outbound, got %+v", generator.lastInput.Messages[1])
	}
	if generator.lastInput.ScenarioNotes == "" {
		t.Fatal("expected scenario notes to be populated")
	}
}
