package ports

import (
	"context"

	"github.com/google/uuid"
)

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
