package service

import (
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/imap/client"
	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/imap/transport"
)

const unchangedAISuggestion = "Ja, we hebben donderdag nog ruimte."

func TestBuildEmailReplyFeedbackParamsCapturesEditedAISuggestion(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	uid := int64(42)
	fromAddress := "customer@example.com"
	fromName := "Robin Klant"
	bodyText := "Kunt u vrijdag langskomen?"

	params, ok := buildEmailReplyFeedbackParams(
		organizationID,
		repository.Account{ID: accountID},
		uid,
		true,
		client.MessageContent{
			FromAddress: &fromAddress,
			FromName:    &fromName,
			Subject:     "Planning",
			Text:        &bodyText,
		},
		transport.ReplyRequest{
			Body:         "Wij kunnen vrijdag om 10:00 langskomen.",
			AISuggestion: strPtr("Wij kunnen morgen langskomen."),
		},
	)

	if !ok {
		t.Fatal("expected email reply feedback params to be created")
	}
	if params.OrganizationID != organizationID || params.AccountID != accountID || params.SourceUID != uid {
		t.Fatalf("unexpected identity fields: %+v", params)
	}
	if params.CustomerEmail != fromAddress {
		t.Fatalf("expected customer email %q, got %q", fromAddress, params.CustomerEmail)
	}
	if params.AIReply == nil || *params.AIReply != "Wij kunnen morgen langskomen." {
		t.Fatalf("expected AI reply to be retained, got %+v", params.AIReply)
	}
	if params.HumanReply != "Wij kunnen vrijdag om 10:00 langskomen." {
		t.Fatalf("unexpected human reply: %+v", params)
	}
	if !params.ReplyAll {
		t.Fatal("expected reply-all flag to be captured")
	}
}

func TestBuildEmailReplyFeedbackParamsCapturesUnchangedSuggestionAsUneditedSend(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	fromAddress := "customer@example.com"
	bodyText := "Hebben jullie nog plek deze week?"

	params, ok := buildEmailReplyFeedbackParams(
		organizationID,
		repository.Account{ID: accountID},
		7,
		false,
		client.MessageContent{
			FromAddress: &fromAddress,
			Subject:     "Beschikbaarheid",
			Text:        &bodyText,
		},
		transport.ReplyRequest{
			Body:         unchangedAISuggestion,
			AISuggestion: strPtr(unchangedAISuggestion),
		},
	)

	if !ok {
		t.Fatal("expected example row to still be captured")
	}
	if params.AIReply == nil || *params.AIReply != unchangedAISuggestion {
		t.Fatalf("expected unchanged AI suggestion to be retained for analytics, got %+v", params.AIReply)
	}
	if params.WasEdited {
		t.Fatalf("expected unchanged AI suggestion to be marked as not edited, got %+v", params)
	}
}

func strPtr(value string) *string {
	return &value
}
