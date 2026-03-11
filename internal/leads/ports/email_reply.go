package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrEmailReplyLeadContextUnavailable = errors.New("email reply lead context unavailable")

type EmailReplyFeedback struct {
	AIReply    string
	HumanReply string
	CreatedAt  time.Time
}

type EmailReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type EmailReplyInput struct {
	OrganizationID uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
	CustomerEmail  string
	CustomerName   string
	Subject        string
	MessageBody    string
	Feedback       []EmailReplyFeedback
	Examples       []EmailReplyExample
}

type EmailReplyGenerator interface {
	SuggestEmailReply(ctx context.Context, input EmailReplyInput) (string, error)
}
