package transport

import (
	"time"

	"github.com/google/uuid"
)

// QuoteStatus defines the status of a quote
type QuoteStatus string

const (
	QuoteStatusDraft    QuoteStatus = "Draft"
	QuoteStatusSent     QuoteStatus = "Sent"
	QuoteStatusAccepted QuoteStatus = "Accepted"
	QuoteStatusRejected QuoteStatus = "Rejected"
	QuoteStatusExpired  QuoteStatus = "Expired"
)

// ── Requests ──────────────────────────────────────────────────────────────────

// QuoteItemRequest is the input for a single line item
type QuoteItemRequest struct {
	Description    string `json:"description" validate:"required"`
	Quantity       string `json:"quantity" validate:"required"`
	UnitPriceCents int64  `json:"unitPriceCents" validate:"min=0"`
	TaxRateBps     int    `json:"taxRateBps" validate:"min=0"`
	IsOptional     bool   `json:"isOptional"`
	IsSelected     bool   `json:"isSelected"`
}

// CreateQuoteRequest is the request body for creating a new quote
type CreateQuoteRequest struct {
	LeadID        uuid.UUID          `json:"leadId" validate:"required"`
	LeadServiceID *uuid.UUID         `json:"leadServiceId"`
	PricingMode   string             `json:"pricingMode" validate:"omitempty,oneof=exclusive inclusive"`
	DiscountType  string             `json:"discountType" validate:"omitempty,oneof=percentage fixed"`
	DiscountValue int64              `json:"discountValue" validate:"min=0"`
	ValidUntil    *time.Time         `json:"validUntil"`
	Notes         string             `json:"notes"`
	Items         []QuoteItemRequest `json:"items" validate:"required,dive"`
}

// UpdateQuoteRequest is the request body for updating a quote
type UpdateQuoteRequest struct {
	PricingMode   *string             `json:"pricingMode" validate:"omitempty,oneof=exclusive inclusive"`
	DiscountType  *string             `json:"discountType" validate:"omitempty,oneof=percentage fixed"`
	DiscountValue *int64              `json:"discountValue" validate:"omitempty,min=0"`
	ValidUntil    *time.Time          `json:"validUntil"`
	Notes         *string             `json:"notes"`
	Items         *[]QuoteItemRequest `json:"items" validate:"omitempty,dive"`
}

// UpdateQuoteStatusRequest is the request body for updating a quote's status
type UpdateQuoteStatusRequest struct {
	Status QuoteStatus `json:"status" validate:"required,oneof=Draft Sent Accepted Rejected Expired"`
}

// QuoteCalculationRequest is the request body for the preview calculation endpoint
type QuoteCalculationRequest struct {
	Items         []QuoteItemRequest `json:"items" validate:"required,dive"`
	PricingMode   string             `json:"pricingMode" validate:"omitempty,oneof=exclusive inclusive"`
	DiscountType  string             `json:"discountType" validate:"omitempty,oneof=percentage fixed"`
	DiscountValue int64              `json:"discountValue" validate:"min=0"`
}

// ListQuotesRequest defines the query parameters for listing quotes
type ListQuotesRequest struct {
	LeadID         string `form:"leadId"`
	Status         string `form:"status" validate:"omitempty,oneof=Draft Sent Accepted Rejected Expired"`
	Search         string `form:"search"`
	CreatedAtFrom  string `form:"createdAtFrom" validate:"omitempty"`
	CreatedAtTo    string `form:"createdAtTo" validate:"omitempty"`
	ValidUntilFrom string `form:"validUntilFrom" validate:"omitempty"`
	ValidUntilTo   string `form:"validUntilTo" validate:"omitempty"`
	TotalFrom      string `form:"totalFrom" validate:"omitempty"`
	TotalTo        string `form:"totalTo" validate:"omitempty"`
	SortBy         string `form:"sortBy" validate:"omitempty,oneof=quoteNumber status total validUntil customerName customerPhone customerAddress createdBy createdAt updatedAt"`
	SortOrder      string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	Page           int    `form:"page" validate:"omitempty,min=1"`
	PageSize       int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
}

// ── Responses ─────────────────────────────────────────────────────────────────

// QuoteItemResponse is the response for a single line item
type QuoteItemResponse struct {
	ID                  uuid.UUID            `json:"id"`
	Description         string               `json:"description"`
	Quantity            string               `json:"quantity"`
	UnitPriceCents      int64                `json:"unitPriceCents"`
	TaxRateBps          int                  `json:"taxRateBps"`
	IsOptional          bool                 `json:"isOptional"`
	IsSelected          bool                 `json:"isSelected"`
	SortOrder           int                  `json:"sortOrder"`
	TotalBeforeTaxCents int64                `json:"totalBeforeTaxCents"`
	TotalTaxCents       int64                `json:"totalTaxCents"`
	LineTotalCents      int64                `json:"lineTotalCents"`
	Annotations         []AnnotationResponse `json:"annotations"`
}

// QuoteResponse is the response for a quote
type QuoteResponse struct {
	ID                         uuid.UUID           `json:"id"`
	QuoteNumber                string              `json:"quoteNumber"`
	LeadID                     uuid.UUID           `json:"leadId"`
	LeadServiceID              *uuid.UUID          `json:"leadServiceId,omitempty"`
	CreatedByID                *uuid.UUID          `json:"createdById,omitempty"`
	CreatedByFirstName         *string             `json:"createdByFirstName,omitempty"`
	CreatedByLastName          *string             `json:"createdByLastName,omitempty"`
	CreatedByEmail             *string             `json:"createdByEmail,omitempty"`
	CustomerFirstName          *string             `json:"customerFirstName,omitempty"`
	CustomerLastName           *string             `json:"customerLastName,omitempty"`
	CustomerPhone              *string             `json:"customerPhone,omitempty"`
	CustomerEmail              *string             `json:"customerEmail,omitempty"`
	CustomerAddressStreet      *string             `json:"customerAddressStreet,omitempty"`
	CustomerAddressHouseNumber *string             `json:"customerAddressHouseNumber,omitempty"`
	CustomerAddressZipCode     *string             `json:"customerAddressZipCode,omitempty"`
	CustomerAddressCity        *string             `json:"customerAddressCity,omitempty"`
	Status                     QuoteStatus         `json:"status"`
	PricingMode                string              `json:"pricingMode"`
	DiscountType               string              `json:"discountType"`
	DiscountValue              int64               `json:"discountValue"`
	SubtotalCents              int64               `json:"subtotalCents"`
	DiscountAmountCents        int64               `json:"discountAmountCents"`
	TaxTotalCents              int64               `json:"taxTotalCents"`
	TotalCents                 int64               `json:"totalCents"`
	ValidUntil                 *time.Time          `json:"validUntil,omitempty"`
	Notes                      *string             `json:"notes,omitempty"`
	Items                      []QuoteItemResponse `json:"items"`
	ViewedAt                   *time.Time          `json:"viewedAt,omitempty"`
	AcceptedAt                 *time.Time          `json:"acceptedAt,omitempty"`
	RejectedAt                 *time.Time          `json:"rejectedAt,omitempty"`
	PDFFileKey                 *string             `json:"pdfFileKey,omitempty"`
	CreatedAt                  time.Time           `json:"createdAt"`
	UpdatedAt                  time.Time           `json:"updatedAt"`
}

// QuoteListResponse is the paginated list response
type QuoteListResponse struct {
	Items      []QuoteResponse `json:"items"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalPages int             `json:"totalPages"`
}

// QuotePreviewLinkResponse is the response for a read-only preview link.
type QuotePreviewLinkResponse struct {
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// VatBreakdown represents a single VAT rate line
type VatBreakdown struct {
	RateBps     int   `json:"rateBps"`
	AmountCents int64 `json:"amountCents"`
}

// CalculatedLineItem is a fully calculated line returned from the preview endpoint
type CalculatedLineItem struct {
	Description         string `json:"description"`
	Quantity            string `json:"quantity"`
	UnitPriceCents      int64  `json:"unitPriceCents"`
	TaxRateBps          int    `json:"taxRateBps"`
	IsOptional          bool   `json:"isOptional"`
	IsSelected          bool   `json:"isSelected"`
	TotalBeforeTaxCents int64  `json:"totalBeforeTaxCents"`
	TotalTaxCents       int64  `json:"totalTaxCents"`
	LineTotalCents      int64  `json:"lineTotalCents"`
}

// QuoteCalculationResponse is the response for the preview calculation
type QuoteCalculationResponse struct {
	Lines               []CalculatedLineItem `json:"lines"`
	SubtotalCents       int64                `json:"subtotalCents"`
	DiscountAmountCents int64                `json:"discountAmountCents"`
	VatTotalCents       int64                `json:"vatTotalCents"`
	VatBreakdown        []VatBreakdown       `json:"vatBreakdown"`
	TotalCents          int64                `json:"totalCents"`
}

// ── Public Quote DTOs ─────────────────────────────────────────────────────────

// AnnotationResponse is the response for a single annotation on a line item.
type AnnotationResponse struct {
	ID         uuid.UUID  `json:"id"`
	ItemID     uuid.UUID  `json:"itemId"`
	AuthorType string     `json:"authorType"`
	AuthorID   *uuid.UUID `json:"authorId,omitempty"`
	Text       string     `json:"text"`
	IsResolved bool       `json:"isResolved"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// PublicQuoteItemResponse is the public-facing response for a line item (includes annotations).
type PublicQuoteItemResponse struct {
	ID                  uuid.UUID            `json:"id"`
	Description         string               `json:"description"`
	Quantity            string               `json:"quantity"`
	UnitPriceCents      int64                `json:"unitPriceCents"`
	TaxRateBps          int                  `json:"taxRateBps"`
	IsOptional          bool                 `json:"isOptional"`
	IsSelected          bool                 `json:"isSelected"`
	SortOrder           int                  `json:"sortOrder"`
	TotalBeforeTaxCents int64                `json:"totalBeforeTaxCents"`
	TotalTaxCents       int64                `json:"totalTaxCents"`
	LineTotalCents      int64                `json:"lineTotalCents"`
	Annotations         []AnnotationResponse `json:"annotations"`
}

// PublicQuoteResponse is the public-facing response for a quote proposal.
type PublicQuoteResponse struct {
	ID                  uuid.UUID                 `json:"id"`
	QuoteNumber         string                    `json:"quoteNumber"`
	Status              QuoteStatus               `json:"status"`
	PricingMode         string                    `json:"pricingMode"`
	OrganizationName    string                    `json:"organizationName"`
	CustomerName        string                    `json:"customerName"`
	DiscountType        string                    `json:"discountType"`
	DiscountValue       int64                     `json:"discountValue"`
	SubtotalCents       int64                     `json:"subtotalCents"`
	DiscountAmountCents int64                     `json:"discountAmountCents"`
	TaxTotalCents       int64                     `json:"taxTotalCents"`
	TotalCents          int64                     `json:"totalCents"`
	VatBreakdown        []VatBreakdown            `json:"vatBreakdown"`
	ValidUntil          *time.Time                `json:"validUntil,omitempty"`
	Notes               *string                   `json:"notes,omitempty"`
	Items               []PublicQuoteItemResponse `json:"items"`
	AcceptedAt          *time.Time                `json:"acceptedAt,omitempty"`
	RejectedAt          *time.Time                `json:"rejectedAt,omitempty"`
	IsReadOnly          bool                      `json:"isReadOnly,omitempty"`
}

// ToggleItemRequest is the request body for toggling an optional item.
type ToggleItemRequest struct {
	IsSelected bool `json:"isSelected"`
}

// ToggleItemResponse is returned after toggling an item, with recalculated totals.
type ToggleItemResponse struct {
	SubtotalCents       int64          `json:"subtotalCents"`
	DiscountAmountCents int64          `json:"discountAmountCents"`
	TaxTotalCents       int64          `json:"taxTotalCents"`
	TotalCents          int64          `json:"totalCents"`
	VatBreakdown        []VatBreakdown `json:"vatBreakdown"`
}

// AnnotateItemRequest is the request body for creating an annotation on a line item.
type AnnotateItemRequest struct {
	Text string `json:"text" validate:"required,min=1,max=2000"`
}

// AcceptQuoteRequest is the request body for accepting a quote.
type AcceptQuoteRequest struct {
	SignatureName string `json:"signatureName" validate:"required,min=1,max=255"`
	SignatureData string `json:"signatureData" validate:"required"`
}

// RejectQuoteRequest is the request body for rejecting a quote.
type RejectQuoteRequest struct {
	Reason string `json:"reason" validate:"max=2000"`
}

// QuoteActivityResponse is the response for a single activity log entry.
type QuoteActivityResponse struct {
	ID        uuid.UUID              `json:"id"`
	EventType string                 `json:"eventType"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
}
