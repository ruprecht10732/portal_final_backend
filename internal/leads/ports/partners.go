package ports

import (
	"context"

	"github.com/google/uuid"
)

type CreateOfferParams struct {
	PartnerID          uuid.UUID
	LeadServiceID      uuid.UUID
	PricingSource      string // "quote" or "estimate"
	CustomerPriceCents int64
	ExpiresInHours     int
	JobSummaryShort    string
}

type CreateOfferResult struct {
	OfferID     uuid.UUID
	PublicToken string
	ExpiresAt   string
}

// PartnerOfferCreator defines the capability to create job offers for partners.
type PartnerOfferCreator interface {
	CreateOffer(ctx context.Context, tenantID uuid.UUID, req CreateOfferParams) (*CreateOfferResult, error)
}
