package transport

import (
	"testing"
	"time"

	"portal_final_backend/internal/identity/repository"

	"github.com/google/uuid"
)

func TestToWhatsAppConversationResponse_AllowsNilLeadID(t *testing.T) {
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

func TestToWhatsAppMessageResponse_MapsLeadID(t *testing.T) {
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
