package transport

import (
	"time"

	"github.com/google/uuid"
)

// --- Internal API (Dispatcher / Agent) ---

// CreateOfferRequest is the internal request to create a partner offer.
type CreateOfferRequest struct {
	PartnerID          uuid.UUID `json:"partnerId" validate:"required"`
	LeadServiceID      uuid.UUID `json:"leadServiceId" validate:"required"`
	PricingSource      string    `json:"pricingSource" validate:"required,oneof=quote estimate"`
	CustomerPriceCents int64     `json:"customerPriceCents" validate:"min=0"`
	ExpiresInHours     int       `json:"expiresInHours" validate:"required,min=1,max=168"`
	JobSummaryShort    string    `json:"jobSummaryShort,omitempty" validate:"omitempty,max=200"`
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

// --- Public API (Vakman-facing) ---

// PublicOfferResponse is the Vakman's view. Hides customer markup.
type PublicOfferResponse struct {
	OfferID          uuid.UUID `json:"offerId"`
	OrganizationName string    `json:"organizationName"`
	JobSummary       string    `json:"jobSummary"`
	JobSummaryShort  *string   `json:"jobSummaryShort,omitempty"`
	City             string    `json:"city"`
	Postcode4        *string   `json:"postcode4,omitempty"`
	Buurtcode        *string   `json:"buurtcode,omitempty"`
	VakmanPriceCents int64     `json:"vakmanPriceCents"`
	PricingSource    string    `json:"pricingSource"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expiresAt"`
	CreatedAt        time.Time `json:"createdAt"`
}

// AcceptOfferRequest is the vakman's acceptance payload.
type AcceptOfferRequest struct {
	InspectionSlots []TimeSlot `json:"inspectionSlots" validate:"required,min=1,dive"`
	JobSlots        []TimeSlot `json:"jobSlots,omitempty" validate:"omitempty,dive"`
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
