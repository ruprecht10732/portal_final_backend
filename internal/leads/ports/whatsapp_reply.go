package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type WhatsAppReplyMessage struct {
	Direction string
	Body      string
	CreatedAt time.Time
}

type WhatsAppReplyInput struct {
	OrganizationID uuid.UUID
	LeadID         uuid.UUID
	ConversationID uuid.UUID
	PhoneNumber    string
	DisplayName    string
	Messages       []WhatsAppReplyMessage
}

type WhatsAppReplyGenerator interface {
	SuggestWhatsAppReply(ctx context.Context, input WhatsAppReplyInput) (string, error)
}
