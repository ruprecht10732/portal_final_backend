// Package ports defines the interfaces that the RAC_leads domain requires from
// external systems. These interfaces form the Anti-Corruption Layer (ACL),
// ensuring the RAC_leads domain only knows about the data it needs, formatted
// the way it wants.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// DraftQuoteItem represents a single line item for the AI-drafted quote.
type DraftQuoteItem struct {
	Description      string
	Quantity         string // e.g. "3", "1"
	UnitPriceCents   int64
	TaxRateBps       int
	IsOptional       bool
	CatalogProductID *uuid.UUID // nil for ad-hoc items
}

// DraftQuoteParams contains everything the leads agent needs to create a draft quote.
type DraftQuoteParams struct {
	LeadID         uuid.UUID
	LeadServiceID  uuid.UUID
	OrganizationID uuid.UUID
	CreatedByID    uuid.UUID // system/agent user ID
	Notes          string
	Items          []DraftQuoteItem
}

// DraftQuoteResult is the minimal response the leads domain needs after a quote
// is successfully drafted.
type DraftQuoteResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

// QuoteDrafter is the ACL interface through which the leads agent can draft
// quotes in the quotes domain. The adapter delegates to the quotes service.
type QuoteDrafter interface {
	// DraftQuote creates a new draft quote and emits the appropriate timeline event.
	DraftQuote(ctx context.Context, params DraftQuoteParams) (*DraftQuoteResult, error)
}
