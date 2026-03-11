package ports

import (
	"context"

	"github.com/google/uuid"
)

type ReplyUserProfile struct {
	ID        uuid.UUID
	Email     string
	FirstName *string
	LastName  *string
}

type ReplyUserReader interface {
	GetUserProfile(ctx context.Context, userID uuid.UUID) (*ReplyUserProfile, error)
}

type ReplyQuoteReader interface {
	GetAcceptedQuote(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (*PublicQuoteSummary, error)
}
