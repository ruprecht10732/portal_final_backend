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

type WhatsAppReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type WhatsAppReplyFeedback struct {
	AIReply    string
	HumanReply string
	CreatedAt  time.Time
}

type WhatsAppReplyInput struct {
	OrganizationID  uuid.UUID
	RequesterUserID uuid.UUID
	LeadID          uuid.UUID
	ConversationID  uuid.UUID
	Scenario        ReplySuggestionScenario
	ScenarioNotes   string
	PhoneNumber     string
	DisplayName     string
	Messages        []WhatsAppReplyMessage
	Examples        []WhatsAppReplyExample
	Feedback        []WhatsAppReplyFeedback
}

type WhatsAppReplyGenerator interface {
	SuggestWhatsAppReply(ctx context.Context, input WhatsAppReplyInput) (string, error)
}
