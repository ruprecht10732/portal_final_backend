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
	Description      string     `json:"description" validate:"required"`
	Quantity         string     `json:"quantity" validate:"required"`
	UnitPriceCents   int64      `json:"unitPriceCents" validate:"min=0"`
	TaxRateBps       int        `json:"taxRateBps" validate:"min=0"`
	IsOptional       bool       `json:"isOptional"`
	IsSelected       bool       `json:"isSelected"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
}

// QuoteAttachmentRequest is the input for a document attachment on a quote.
type QuoteAttachmentRequest struct {
	Filename         string     `json:"filename" validate:"required,min=1,max=500"`
	FileKey          string     `json:"fileKey" validate:"required,min=1,max=1000"`
	Source           string     `json:"source" validate:"required,oneof=catalog manual"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
	Enabled          bool       `json:"enabled"`
	SortOrder        int        `json:"sortOrder" validate:"min=0"`
}

// QuoteURLRequest is the input for a URL attachment on a quote.
type QuoteURLRequest struct {
	Label            string     `json:"label" validate:"required,min=1,max=500"`
	Href             string     `json:"href" validate:"required,url,max=2000"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
}

// CreateQuoteRequest is the request body for creating a new quote
type CreateQuoteRequest struct {
	LeadID              uuid.UUID                `json:"leadId" validate:"required"`
	LeadServiceID       *uuid.UUID               `json:"leadServiceId"`
	PricingMode         string                   `json:"pricingMode" validate:"omitempty,oneof=exclusive inclusive"`
	DiscountType        string                   `json:"discountType" validate:"omitempty,oneof=percentage fixed"`
	DiscountValue       int64                    `json:"discountValue" validate:"min=0"`
	ValidUntil          *time.Time               `json:"validUntil"`
	Notes               string                   `json:"notes"`
	Items               []QuoteItemRequest       `json:"items" validate:"required,dive"`
	Attachments         []QuoteAttachmentRequest `json:"attachments" validate:"omitempty,dive"`
	URLs                []QuoteURLRequest        `json:"urls" validate:"omitempty,dive"`
	FinancingDisclaimer bool                     `json:"financingDisclaimer"`
}

// UpdateQuoteRequest is the request body for updating a quote
type UpdateQuoteRequest struct {
	PricingMode         *string                   `json:"pricingMode" validate:"omitempty,oneof=exclusive inclusive"`
	DiscountType        *string                   `json:"discountType" validate:"omitempty,oneof=percentage fixed"`
	DiscountValue       *int64                    `json:"discountValue" validate:"omitempty,min=0"`
	ValidUntil          *time.Time                `json:"validUntil"`
	Notes               *string                   `json:"notes"`
	Items               *[]QuoteItemRequest       `json:"items" validate:"omitempty,dive"`
	Attachments         *[]QuoteAttachmentRequest `json:"attachments" validate:"omitempty,dive"`
	URLs                *[]QuoteURLRequest        `json:"urls" validate:"omitempty,dive"`
	FinancingDisclaimer *bool                     `json:"financingDisclaimer"`
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

// ListPendingApprovalsRequest defines query parameters for draft-approval queue.
type ListPendingApprovalsRequest struct {
	Page     int `form:"page" validate:"omitempty,min=1"`
	PageSize int `form:"pageSize" validate:"omitempty,min=1,max=100"`
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
	CatalogProductID    *uuid.UUID           `json:"catalogProductId,omitempty"`
	Annotations         []AnnotationResponse `json:"annotations"`
}

// QuoteAttachmentResponse is the response for a document attachment.
type QuoteAttachmentResponse struct {
	ID               uuid.UUID  `json:"id"`
	Filename         string     `json:"filename"`
	FileKey          string     `json:"fileKey"`
	Source           string     `json:"source"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
	Enabled          bool       `json:"enabled"`
	SortOrder        int        `json:"sortOrder"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// QuoteURLResponse is the response for a URL attachment.
type QuoteURLResponse struct {
	ID               uuid.UUID  `json:"id"`
	Label            string     `json:"label"`
	Href             string     `json:"href"`
	Accepted         bool       `json:"accepted"`
	CatalogProductID *uuid.UUID `json:"catalogProductId,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// ── Presigned Upload ─────────────────────────────────────────────────────────

// PresignAttachmentUploadRequest is the request for generating a presigned URL
// for uploading a manual PDF attachment to a quote.
type PresignAttachmentUploadRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,eq=application/pdf"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// PresignedUploadResponse is the generic presigned upload URL response.
type PresignedUploadResponse struct {
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
	ExpiresAt int64  `json:"expiresAt"`
}

// PresignedDownloadResponse is the presigned URL for downloading an attachment.
type PresignedDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// QuoteResponse is the response for a quote
type QuoteResponse struct {
	ID                         uuid.UUID                 `json:"id"`
	QuoteNumber                string                    `json:"quoteNumber"`
	LeadID                     uuid.UUID                 `json:"leadId"`
	LeadServiceID              *uuid.UUID                `json:"leadServiceId,omitempty"`
	CreatedByID                *uuid.UUID                `json:"createdById,omitempty"`
	CreatedByFirstName         *string                   `json:"createdByFirstName,omitempty"`
	CreatedByLastName          *string                   `json:"createdByLastName,omitempty"`
	CreatedByEmail             *string                   `json:"createdByEmail,omitempty"`
	CustomerFirstName          *string                   `json:"customerFirstName,omitempty"`
	CustomerLastName           *string                   `json:"customerLastName,omitempty"`
	CustomerPhone              *string                   `json:"customerPhone,omitempty"`
	CustomerEmail              *string                   `json:"customerEmail,omitempty"`
	CustomerAddressStreet      *string                   `json:"customerAddressStreet,omitempty"`
	CustomerAddressHouseNumber *string                   `json:"customerAddressHouseNumber,omitempty"`
	CustomerAddressZipCode     *string                   `json:"customerAddressZipCode,omitempty"`
	CustomerAddressCity        *string                   `json:"customerAddressCity,omitempty"`
	Status                     QuoteStatus               `json:"status"`
	PricingMode                string                    `json:"pricingMode"`
	DiscountType               string                    `json:"discountType"`
	DiscountValue              int64                     `json:"discountValue"`
	SubtotalCents              int64                     `json:"subtotalCents"`
	DiscountAmountCents        int64                     `json:"discountAmountCents"`
	TaxTotalCents              int64                     `json:"taxTotalCents"`
	TotalCents                 int64                     `json:"totalCents"`
	ValidUntil                 *time.Time                `json:"validUntil,omitempty"`
	Notes                      *string                   `json:"notes,omitempty"`
	Items                      []QuoteItemResponse       `json:"items"`
	Attachments                []QuoteAttachmentResponse `json:"attachments"`
	URLs                       []QuoteURLResponse        `json:"urls"`
	ViewedAt                   *time.Time                `json:"viewedAt,omitempty"`
	AcceptedAt                 *time.Time                `json:"acceptedAt,omitempty"`
	RejectedAt                 *time.Time                `json:"rejectedAt,omitempty"`
	PDFFileKey                 *string                   `json:"pdfFileKey,omitempty"`
	FinancingDisclaimer        bool                      `json:"financingDisclaimer"`
	CreatedAt                  time.Time                 `json:"createdAt"`
	UpdatedAt                  time.Time                 `json:"updatedAt"`
}

// QuoteListResponse is the paginated list response
type QuoteListResponse struct {
	Items      []QuoteResponse `json:"items"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalPages int             `json:"totalPages"`
}

// PendingApprovalItem represents one draft quote ready for agent review.
type PendingApprovalItem struct {
	QuoteID         uuid.UUID `json:"quoteId"`
	LeadID          uuid.UUID `json:"leadId"`
	QuoteNumber     string    `json:"quoteNumber"`
	ConsumerName    string    `json:"consumerName"`
	TotalCents      int64     `json:"totalCents"`
	ConfidenceScore *int      `json:"confidenceScore,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

// PendingApprovalsResponse is the paginated draft approval queue response.
type PendingApprovalsResponse struct {
	Items      []PendingApprovalItem `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
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
	Attachments         []QuoteAttachmentResponse `json:"attachments"`
	URLs                []QuoteURLResponse        `json:"urls"`
	AcceptedAt          *time.Time                `json:"acceptedAt,omitempty"`
	RejectedAt          *time.Time                `json:"rejectedAt,omitempty"`
	FinancingDisclaimer bool                      `json:"financingDisclaimer"`
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

// GenerateQuoteRequest is the request body for AI-generated quote creation.
type GenerateQuoteRequest struct {
	LeadID        uuid.UUID  `json:"leadId" validate:"required"`
	LeadServiceID *uuid.UUID `json:"leadServiceId"`
	Prompt        string     `json:"prompt" validate:"required,min=5,max=2000"`
	QuoteID       *uuid.UUID `json:"quoteId"` // If set, update the existing quote instead of creating a new one
}

// GenerateQuoteAcceptedResponse is returned when async generation is started.
type GenerateQuoteAcceptedResponse struct {
	JobID  uuid.UUID `json:"jobId"`
	Status string    `json:"status"`
}

// GenerateQuoteJobResponse returns current job state for async generation.
type GenerateQuoteJobResponse struct {
	JobID           uuid.UUID  `json:"jobId"`
	Status          string     `json:"status"`
	Step            string     `json:"step"`
	ProgressPercent int        `json:"progressPercent"`
	Error           *string    `json:"error,omitempty"`
	QuoteID         *uuid.UUID `json:"quoteId,omitempty"`
	QuoteNumber     *string    `json:"quoteNumber,omitempty"`
	ItemCount       *int       `json:"itemCount,omitempty"`
	LeadID          uuid.UUID  `json:"leadId"`
	LeadServiceID   uuid.UUID  `json:"leadServiceId"`
	StartedAt       time.Time  `json:"startedAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

// QuoteActivityResponse is the response for a single activity log entry.
type QuoteActivityResponse struct {
	ID        uuid.UUID              `json:"id"`
	EventType string                 `json:"eventType"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
}

type ProviderIntegrationStatusResponse struct {
	Provider    string     `json:"provider"`
	IsConnected bool       `json:"isConnected"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
}

type QuoteExportResponse struct {
	QuoteID     uuid.UUID `json:"quoteId"`
	Provider    string    `json:"provider"`
	ExternalID  string    `json:"externalId"`
	ExternalURL string    `json:"externalUrl,omitempty"`
	State       string    `json:"state"`
	ExportedAt  time.Time `json:"exportedAt"`
}

type QuoteExportStatusResponse struct {
	QuoteID     uuid.UUID  `json:"quoteId"`
	Provider    string     `json:"provider"`
	IsExported  bool       `json:"isExported"`
	ExternalID  *string    `json:"externalId,omitempty"`
	ExternalURL *string    `json:"externalUrl,omitempty"`
	State       *string    `json:"state,omitempty"`
	ExportedAt  *time.Time `json:"exportedAt,omitempty"`
}

type BulkQuoteExportRequest struct {
	QuoteIDs []uuid.UUID `json:"quoteIds" validate:"required,min=1,max=100,dive,required"`
}

type BulkQuoteExportItem struct {
	QuoteID     uuid.UUID  `json:"quoteId"`
	Provider    string     `json:"provider"`
	Status      string     `json:"status"`
	ExternalID  *string    `json:"externalId,omitempty"`
	ExternalURL *string    `json:"externalUrl,omitempty"`
	State       *string    `json:"state,omitempty"`
	ExportedAt  *time.Time `json:"exportedAt,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

type BulkQuoteExportResponse struct {
	Items []BulkQuoteExportItem `json:"items"`
}

type MoneybirdAuthorizeURLResponse struct {
	Provider     string `json:"provider"`
	AuthorizeURL string `json:"authorizeUrl"`
}

type MoneybirdCallbackResponse struct {
	Provider         string `json:"provider"`
	IsConnected      bool   `json:"isConnected"`
	AdministrationID string `json:"administrationId,omitempty"`
}
