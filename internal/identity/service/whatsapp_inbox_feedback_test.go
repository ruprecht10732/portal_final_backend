package service

import (
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/identity/repository"
)

func TestBuildWhatsAppReplyFeedbackParamsCapturesEditedAISuggestion(t *testing.T) {
	organizationID := uuid.New()
	conversationID := uuid.New()
	leadID := uuid.New()

	params, ok := buildWhatsAppReplyFeedbackParams(repository.WhatsAppConversation{
		OrganizationID: organizationID,
		ID:             conversationID,
		LeadID:         &leadID,
	}, SendWhatsAppConversationMessageInput{
		Type:         "text",
		Body:         "Aangepaste versie voor de klant",
		AISuggestion: "Originele AI-versie",
	})

	if !ok {
		t.Fatal("expected feedback params to be created")
	}
	if params.OrganizationID != organizationID || params.ConversationID != conversationID || params.LeadID != leadID {
		t.Fatalf("unexpected identity fields: %+v", params)
	}
	if params.AIReply != "Originele AI-versie" || params.HumanReply != "Aangepaste versie voor de klant" {
		t.Fatalf("unexpected feedback payload: %+v", params)
	}
}

func TestBuildWhatsAppReplyFeedbackParamsSkipsUnsupportedMessagesOrMissingAI(t *testing.T) {
	leadID := uuid.New()
	conversation := repository.WhatsAppConversation{
		OrganizationID: uuid.New(),
		ID:             uuid.New(),
		LeadID:         &leadID,
	}

	testCases := []SendWhatsAppConversationMessageInput{
		{Type: "image", Body: "Bijschrift", AISuggestion: "AI tekst"},
		{Type: "text", Body: "Menselijke tekst", AISuggestion: "   "},
	}

	for _, input := range testCases {
		if _, ok := buildWhatsAppReplyFeedbackParams(conversation, input); ok {
			t.Fatalf("expected no feedback params for input %+v", input)
		}
	}
}

func TestBuildWhatsAppReplyFeedbackParamsCapturesUnchangedAISuggestionAsUneditedSend(t *testing.T) {
	leadID := uuid.New()
	conversation := repository.WhatsAppConversation{OrganizationID: uuid.New(), ID: uuid.New(), LeadID: &leadID}

	params, ok := buildWhatsAppReplyFeedbackParams(conversation, SendWhatsAppConversationMessageInput{
		Type:         "text",
		Body:         "Zelfde tekst",
		AISuggestion: "Zelfde tekst",
		Scenario:     "quote_reminder",
	})
	if !ok {
		t.Fatal("expected unchanged AI draft to still be captured for analytics")
	}
	if params.WasEdited {
		t.Fatalf("expected unchanged AI draft to be marked as not edited, got %+v", params)
	}
	if params.Scenario != "quote_reminder" {
		t.Fatalf("expected scenario to be preserved, got %+v", params)
	}
}
