package transport

import (
	"testing"
	"time"

	"portal_final_backend/internal/identity/repository"

	"github.com/google/uuid"
)

func TestToWhatsAppConversationResponseAllowsNilLeadID(t *testing.T) {
	timestamp := time.Date(2026, time.March, 10, 17, 16, 21, 0, time.UTC)
	conversation := repository.WhatsAppConversation{
		ID:                   uuid.New(),
		LeadID:               nil,
		PhoneNumber:          "+31612345678",
		DisplayName:          "Robin",
		LastMessagePreview:   "Hello",
		LastMessageAt:        timestamp,
		LastMessageDirection: "inbound",
		LastMessageStatus:    "received",
		UnreadCount:          2,
		CreatedAt:            timestamp,
		UpdatedAt:            timestamp,
	}

	response := ToWhatsAppConversationResponse(conversation)

	if response.LeadID != nil {
		t.Fatalf("expected nil lead ID, got %v", *response.LeadID)
	}
}

func TestToWhatsAppMessageResponseMapsLeadID(t *testing.T) {
	timestamp := time.Date(2026, time.March, 10, 17, 16, 21, 0, time.UTC)
	leadID := uuid.New()
	message := repository.WhatsAppMessage{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		LeadID:         &leadID,
		Direction:      "outbound",
		Status:         "sent",
		PhoneNumber:    "+31612345678",
		Body:           "Hi",
		CreatedAt:      timestamp,
	}

	response := ToWhatsAppMessageResponse(message)

	if response.LeadID == nil {
		t.Fatal("expected lead ID to be set")
	}
	if *response.LeadID != leadID.String() {
		t.Fatalf("expected lead ID %s, got %s", leadID.String(), *response.LeadID)
	}
}

func TestWithWhatsAppConversationChatJIDSetsTrimmedValue(t *testing.T) {
	conversation := WhatsAppConversationResponse{ID: uuid.New().String()}

	response := WithWhatsAppConversationChatJID(conversation, " 31612345678@s.whatsapp.net ")

	if response.ChatJID == nil {
		t.Fatal("expected chatJid to be set")
	}
	if *response.ChatJID != "31612345678@s.whatsapp.net" {
		t.Fatalf("expected trimmed chatJid, got %q", *response.ChatJID)
	}
}

func TestWithWhatsAppConversationChatJIDClearsEmptyValue(t *testing.T) {
	chatJID := "31612345678@s.whatsapp.net"
	conversation := WhatsAppConversationResponse{ID: uuid.New().String(), ChatJID: &chatJID}

	response := WithWhatsAppConversationChatJID(conversation, "   ")

	if response.ChatJID != nil {
		t.Fatalf("expected empty chatJid to clear existing value, got %q", *response.ChatJID)
	}
}
