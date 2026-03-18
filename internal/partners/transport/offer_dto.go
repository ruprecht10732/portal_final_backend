package transport

import (
	"time"

	"github.com/google/uuid"
)

// --- Internal API (Dispatcher / Agent) ---

// CreateOfferFromQuoteRequest creates a partner offer from a specific quote.
// The backend derives leadServiceId and customerPriceCents from the quote.
type CreateOfferFromQuoteRequest struct {
	PartnerID          uuid.UUID   `json:"partnerId" validate:"required"`
	QuoteID            uuid.UUID   `json:"quoteId" validate:"required"`
	ExpiresInHours     int         `json:"expiresInHours" validate:"required,min=1,max=12"`
	JobSummaryShort    string      `json:"jobSummaryShort,omitempty" validate:"omitempty,max=200"`
	MarginBasisPoints  *int        `json:"marginBasisPoints,omitempty" validate:"omitempty,min=0,max=5000"`
	VakmanPriceCents   *int64      `json:"vakmanPriceCents,omitempty" validate:"omitempty,min=0"`
	SelectedItemIDs    []uuid.UUID `json:"selectedItemIds,omitempty" validate:"omitempty,dive,uuid"`
	RequiresInspection *bool       `json:"requiresInspection,omitempty"`
}

// CreateOfferResponse is returned after successfully creating an offer.
type CreateOfferResponse struct {
	ID               uuid.UUID `json:"id"`
	PublicToken      string    `json:"publicToken"`
	VakmanPriceCents int64     `json:"vakmanPriceCents"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

// OfferResponse is the admin/agent view of an offer.
type OfferResponse struct {
	ID                 uuid.UUID  `json:"id"`
	PartnerID          uuid.UUID  `json:"partnerId"`
	PartnerName        string     `json:"partnerName"`
	ServiceType        *string    `json:"serviceType,omitempty"`
	ServiceTypeID      *uuid.UUID `json:"serviceTypeId,omitempty"`
	LeadCity           *string    `json:"leadCity,omitempty"`
	LeadServiceID      uuid.UUID  `json:"leadServiceId"`
	PricingSource      string     `json:"pricingSource"`
	CustomerPriceCents int64      `json:"customerPriceCents"`
	VakmanPriceCents   int64      `json:"vakmanPriceCents"`
	Status             string     `json:"status"`
	PublicToken        string     `json:"publicToken"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	AcceptedAt         *time.Time `json:"acceptedAt,omitempty"`
	RejectedAt         *time.Time `json:"rejectedAt,omitempty"`
	RejectionReason    string     `json:"rejectionReason,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// ListOffersResponse is the paginated list of offers for a lead service.
type ListOffersResponse struct {
	Items []OfferResponse `json:"items"`
}

// ListOffersRequest is the admin/agent query for the global offers overview.
// Mirrors other list endpoints: search + paging + sort + filters.
type ListOffersRequest struct {
	Search        string `form:"search" validate:"max=100"`
	Page          int    `form:"page" validate:"omitempty,min=1"`
	PageSize      int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
	SortBy        string `form:"sortBy" validate:"omitempty,oneof=createdAt expiresAt status partnerName serviceType vakmanPriceCents customerPriceCents"`
	SortOrder     string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	Status        string `form:"status" validate:"omitempty,oneof=pending sent accepted rejected expired"`
	PartnerID     string `form:"partnerId" validate:"omitempty,uuid"`
	LeadServiceID string `form:"leadServiceId" validate:"omitempty,uuid"`
	ServiceTypeID string `form:"serviceTypeId" validate:"omitempty,uuid"`
}

// OfferListResponse is the paginated list response for the global offers overview.
type OfferListResponse struct {
	Items      []OfferResponse `json:"items"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalPages int             `json:"totalPages"`
}

// --- Public API (Vakman-facing) ---

// PublicOfferResponse is the Vakman's view. Hides customer markup.
type PublicOfferResponse struct {
	OfferID            uuid.UUID             `json:"offerId"`
	OrganizationName   string                `json:"organizationName"`
	JobSummary         string                `json:"jobSummary"`
	JobSummaryShort    *string               `json:"jobSummaryShort,omitempty"`
	BuilderSummary     *string               `json:"builderSummary,omitempty"`
	City               string                `json:"city"`
	Postcode4          *string               `json:"postcode4,omitempty"`
	Buurtcode          *string               `json:"buurtcode,omitempty"`
	ConstructionYear   *int                  `json:"constructionYear,omitempty"`
	ScopeAssessment    *string               `json:"scopeAssessment,omitempty"`
	UrgencyLevel       *string               `json:"urgencyLevel,omitempty"`
	VakmanPriceCents   int64                 `json:"vakmanPriceCents"`
	PricingSource      string                `json:"pricingSource"`
	Status             string                `json:"status"`
	RequiresInspection bool                  `json:"requiresInspection"`
	ExpiresAt          time.Time             `json:"expiresAt"`
	CreatedAt          time.Time             `json:"createdAt"`
	LineItems          []PublicOfferLineItem `json:"lineItems,omitempty"`
	Photos             []OfferPhotoRef       `json:"photos,omitempty"`
}

type PublicOfferLineItem struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
}

type OfferPhotoRef struct {
	ID          uuid.UUID `json:"id"`
	FileName    string    `json:"fileName"`
	ContentType string    `json:"contentType"`
}

// AcceptOfferRequest is the vakman's acceptance payload.
type AcceptOfferRequest struct {
	InspectionSlots    []TimeSlot `json:"inspectionSlots" validate:"omitempty,dive"`
	JobSlots           []TimeSlot `json:"jobSlots,omitempty" validate:"omitempty,dive"`
	SignerFullName     string     `json:"signerFullName,omitempty" validate:"omitempty,max=200"`
	SignerBusinessName string     `json:"signerBusinessName,omitempty" validate:"omitempty,max=200"`
	SignerAddress      string     `json:"signerAddress,omitempty" validate:"omitempty,max=500"`
	SignatureData      string     `json:"signatureData,omitempty"`
}

// RejectOfferRequest is the vakman's rejection payload.
type RejectOfferRequest struct {
	Reason string `json:"reason" validate:"omitempty,max=500"`
}

// TimeSlot represents a block of availability.
type TimeSlot struct {
	Start time.Time `json:"start" validate:"required"`
	End   time.Time `json:"end" validate:"required,gtfield=Start"`
}

// OfferDetailResponse is the full admin view of an accepted/rejected offer.
// It includes what was sent (line items, photos) and what was received (availability, signer).
type OfferDetailResponse struct {
	ID                 uuid.UUID `json:"id"`
	PartnerID          uuid.UUID `json:"partnerId"`
	PartnerName        string    `json:"partnerName"`
	LeadServiceID      uuid.UUID `json:"leadServiceId"`
	ServiceType        string    `json:"serviceType,omitempty"`
	LeadCity           string    `json:"leadCity,omitempty"`
	Status             string    `json:"status"`
	RequiresInspection bool      `json:"requiresInspection"`
	PublicToken        string    `json:"publicToken"`
	// What was sent
	VakmanPriceCents   int64                 `json:"vakmanPriceCents"`
	CustomerPriceCents int64                 `json:"customerPriceCents"`
	JobSummaryShort    *string               `json:"jobSummaryShort,omitempty"`
	BuilderSummary     *string               `json:"builderSummary,omitempty"`
	LineItems          []OfferDetailLineItem `json:"lineItems,omitempty"`
	Photos             []OfferPhotoRef       `json:"photos,omitempty"`
	ExpiresAt          time.Time             `json:"expiresAt"`
	CreatedAt          time.Time             `json:"createdAt"`
	// What was received (acceptance)
	AcceptedAt         *time.Time `json:"acceptedAt,omitempty"`
	RejectedAt         *time.Time `json:"rejectedAt,omitempty"`
	RejectionReason    *string    `json:"rejectionReason,omitempty"`
	InspectionSlots    []TimeSlot `json:"inspectionSlots,omitempty"`
	JobSlots           []TimeSlot `json:"jobSlots,omitempty"`
	SignerName         *string    `json:"signerName,omitempty"`
	SignerBusinessName *string    `json:"signerBusinessName,omitempty"`
	SignerAddress      *string    `json:"signerAddress,omitempty"`
	// Document
	PDFFileKey *string `json:"pdfFileKey,omitempty"`
}

// OfferDetailLineItem is a full line item in the detail view (includes pricing).
type OfferDetailLineItem struct {
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	LineTotalCents int64  `json:"lineTotalCents"`
}
