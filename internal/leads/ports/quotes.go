// Package ports defines the interfaces that the RAC_leads domain requires from
// external systems. These interfaces form the Anti-Corruption Layer (ACL),
// ensuring the RAC_leads domain only knows about the data it needs, formatted
// the way it wants.
package ports

import (
	"context"
	"time"

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

// DraftQuoteAttachment represents a catalog document to auto-attach to the AI-drafted quote.
type DraftQuoteAttachment struct {
	Filename         string
	FileKey          string
	Source           string     // "catalog"
	CatalogProductID *uuid.UUID // originating product
}

// DraftQuoteURL represents a catalog URL to auto-attach to the AI-drafted quote.
type DraftQuoteURL struct {
	Label            string
	Href             string
	CatalogProductID *uuid.UUID // originating product
}

// DraftQuoteParams contains everything the leads agent needs to create a draft quote.
type DraftQuoteParams struct {
	QuoteID        *uuid.UUID // If set, update the existing quote instead of creating a new one
	LeadID         uuid.UUID
	LeadServiceID  uuid.UUID
	OrganizationID uuid.UUID
	CreatedByID    uuid.UUID // system/agent user ID
	Notes          string
	Items          []DraftQuoteItem
	Attachments    []DraftQuoteAttachment
	URLs           []DraftQuoteURL
}

// DraftQuoteResult is the minimal response the leads domain needs after a quote
// is successfully drafted.
type DraftQuoteResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

type QuoteAIReviewFinding struct {
	Code      string
	Message   string
	Severity  string
	ItemIndex *int
}

const (
	QuoteAIReviewDecisionApproved      = "approved"
	QuoteAIReviewDecisionNeedsRepair   = "needs_repair"
	QuoteAIReviewDecisionRequiresHuman = "requires_human"
)

type RecordQuoteAIReviewParams struct {
	QuoteID        uuid.UUID
	OrganizationID uuid.UUID
	Decision       string
	Summary        string
	Findings       []QuoteAIReviewFinding
	Signals        []string
	AttemptCount   int
	RunID          *string
	ReviewerName   *string
	ModelName      *string
}

type QuoteAIReviewResult struct {
	ReviewID     uuid.UUID
	QuoteID      uuid.UUID
	Decision     string
	Summary      string
	AttemptCount int
	CreatedAt    time.Time
}

// QuoteDrafter is the ACL interface through which the leads agent can draft
// quotes in the quotes domain. The adapter delegates to the quotes service.
type QuoteDrafter interface {
	// DraftQuote creates a new draft quote and emits the appropriate timeline event.
	DraftQuote(ctx context.Context, params DraftQuoteParams) (*DraftQuoteResult, error)
	RecordQuoteAIReview(ctx context.Context, params RecordQuoteAIReviewParams) (*QuoteAIReviewResult, error)
}
