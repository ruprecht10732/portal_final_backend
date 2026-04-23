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

// ──────────────────────────────────────────────────
// Offer summary
// ──────────────────────────────────────────────────

// OfferSummaryItem is a minimal summary line item.
type OfferSummaryItem struct {
	Description string
	Quantity    string
}

// OfferSummaryInput contains non-PII fields for summary generation.
type OfferSummaryInput struct {
	LeadID        uuid.UUID
	LeadServiceID uuid.UUID
	ServiceType   string
	Scope         *string
	UrgencyLevel  *string
	Items         []OfferSummaryItem
}

// OfferSummaryGenerator produces a markdown summary for partner offers.
type OfferSummaryGenerator interface {
	GenerateOfferSummary(ctx context.Context, tenantID uuid.UUID, input OfferSummaryInput) (string, error)
}
