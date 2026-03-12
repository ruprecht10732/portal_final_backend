package service

import (
	"context"
	"errors"
	"testing"

	"portal_final_backend/internal/identity/repository"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

func TestClearStaleWhatsAppConversationLeadRemovesLeadIDAndPersistsCleanup(t *testing.T) {
	t.Parallel()

	conversationID := uuid.New()
	organizationID := uuid.New()
	leadID := uuid.New()
	conversation := &repository.WhatsAppConversation{ID: conversationID, LeadID: &leadID}

	called := false
	clearStaleWhatsAppConversationLead(context.Background(), organizationID, conversation, func(ctx context.Context, gotOrganizationID, gotConversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		called = true
		if gotOrganizationID != organizationID {
			t.Fatalf("expected organization %s, got %s", organizationID, gotOrganizationID)
		}
		if gotConversationID != conversationID {
			t.Fatalf("expected conversation %s, got %s", conversationID, gotConversationID)
		}
		if gotLeadID != nil {
			t.Fatalf("expected cleanup to clear lead id, got %v", *gotLeadID)
		}
		return repository.WhatsAppConversation{ID: conversationID, OrganizationID: organizationID, LeadID: nil}, nil
	})
	if !called {
		t.Fatal("expected cleanup function to be called")
	}
	if conversation.LeadID != nil {
		t.Fatalf("expected in-memory lead id to be cleared, got %v", *conversation.LeadID)
	}
}

func TestClearStaleWhatsAppConversationLeadClearsCurrentResponseEvenWhenCleanupFails(t *testing.T) {
	t.Parallel()

	leadID := uuid.New()
	conversation := &repository.WhatsAppConversation{ID: uuid.New(), LeadID: &leadID}

	clearStaleWhatsAppConversationLead(context.Background(), uuid.New(), conversation, func(ctx context.Context, organizationID, conversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		return repository.WhatsAppConversation{}, errors.New("database unavailable")
	})
	if conversation.LeadID != nil {
		t.Fatalf("expected in-memory lead id to stay cleared on cleanup failure, got %v", *conversation.LeadID)
	}
}

func TestClearStaleWhatsAppConversationLeadNoopsWithoutLeadID(t *testing.T) {
	t.Parallel()

	conversation := &repository.WhatsAppConversation{ID: uuid.New()}
	called := false
	clearStaleWhatsAppConversationLead(context.Background(), uuid.New(), conversation, func(ctx context.Context, organizationID, conversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		called = true
		return repository.WhatsAppConversation{}, nil
	})
	if called {
		t.Fatal("expected no cleanup call when no lead id is present")
	}
}

func TestAppErrIsRecognizesNotFound(t *testing.T) {
	t.Parallel()

	if !apperr.Is(apperr.NotFound("lead not found"), apperr.KindNotFound) {
		t.Fatal("expected apperr.Is to recognize not found errors")
	}
}