package ports

import (
	"context"

	"github.com/google/uuid"
)

type CreateOfferFromQuoteParams struct {
	PartnerID         uuid.UUID
	QuoteID           uuid.UUID
	ExpiresInHours    int
	JobSummaryShort   string
	MarginBasisPoints *int
	VakmanPriceCents  *int64
	SelectedItemIDs   []uuid.UUID
}

type CreateOfferResult struct {
	OfferID     uuid.UUID
	PublicToken string
	ExpiresAt   string
}

// PartnerOfferCreator defines the capability to create job offers for partners.
type PartnerOfferCreator interface {
	CreateOfferFromQuote(ctx context.Context, tenantID uuid.UUID, req CreateOfferFromQuoteParams) (*CreateOfferResult, error)
}
