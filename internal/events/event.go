// Package events provides domain event definitions for decoupled,
// event-driven communication between modules.
// Infrastructure (Bus, Handler) is in platform/events.
package events

import (
	"portal_final_backend/platform/events"

	"github.com/google/uuid"
)

// Re-export platform types for convenience
type (
	Event       = events.Event
	Bus         = events.Bus
	Handler     = events.Handler
	HandlerFunc = events.HandlerFunc
	BaseEvent   = events.BaseEvent
)

// Re-export platform functions
var NewBaseEvent = events.NewBaseEvent

// =============================================================================
// Auth Domain Events
// =============================================================================

// UserSignedUp is published when a new user successfully registers.
type UserSignedUp struct {
	BaseEvent
	UserID      uuid.UUID `json:"userId"`
	Email       string    `json:"email"`
	VerifyToken string    `json:"verifyToken"`
}

func (e UserSignedUp) EventName() string { return "auth.user.signed_up" }

// EmailVerificationRequested is published when a user needs to verify their email.
type EmailVerificationRequested struct {
	BaseEvent
	UserID      uuid.UUID `json:"userId"`
	Email       string    `json:"email"`
	VerifyToken string    `json:"verifyToken"`
}

func (e EmailVerificationRequested) EventName() string { return "auth.email.verification_requested" }

// PasswordResetRequested is published when a user requests a password reset.
type PasswordResetRequested struct {
	BaseEvent
	UserID     uuid.UUID `json:"userId"`
	Email      string    `json:"email"`
	ResetToken string    `json:"resetToken"`
}

func (e PasswordResetRequested) EventName() string { return "auth.password.reset_requested" }

// =============================================================================
// Leads Domain Events
// =============================================================================

// LeadCreated is published when a new lead is created.
type LeadCreated struct {
	BaseEvent
	LeadID          uuid.UUID  `json:"leadId"`
	TenantID        uuid.UUID  `json:"tenantId"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
	ServiceType     string     `json:"serviceType"`
	ConsumerName    string     `json:"consumerName"`
	ConsumerPhone   string     `json:"consumerPhone"`
}

func (e LeadCreated) EventName() string { return "RAC_leads.lead.created" }

// LeadAssigned is published when a lead is assigned to an agent.
type LeadAssigned struct {
	BaseEvent
	LeadID        uuid.UUID  `json:"leadId"`
	PreviousAgent *uuid.UUID `json:"previousAgent,omitempty"`
	NewAgent      *uuid.UUID `json:"newAgent,omitempty"`
	AssignedByID  uuid.UUID  `json:"assignedById"`
}

func (e LeadAssigned) EventName() string { return "RAC_leads.lead.assigned" }

// LeadServiceAdded is published when a new service is added to an existing lead.
type LeadServiceAdded struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	ServiceType   string    `json:"serviceType"`
}

func (e LeadServiceAdded) EventName() string { return "leads.service.added" }

// LeadDataChanged is published when a user updates lead data such as notes or call logs.
type LeadDataChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Source        string    `json:"source"` // "call_log", "note", "user_update"
}

func (e LeadDataChanged) EventName() string { return "leads.data.changed" }

// PipelineStageChanged is published when the pipeline stage for a lead service changes.
type PipelineStageChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	OldStage      string    `json:"oldStage"`
	NewStage      string    `json:"newStage"`
}

func (e PipelineStageChanged) EventName() string { return "leads.pipeline.changed" }

// ManualInterventionRequired is published when a lead service requires manual review.
// This occurs when automated processing fails or identifies edge cases that need human attention.
type ManualInterventionRequired struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Reason        string    `json:"reason"` // "no_partners_found", "estimation_ambiguous", "special_requirements"
	Context       string    `json:"context,omitempty"`
}

func (e ManualInterventionRequired) EventName() string { return "leads.manual_intervention.required" }

// =============================================================================
// Identity Domain Events
// =============================================================================

// OrganizationInviteCreated is published when an organization invite is created.
type OrganizationInviteCreated struct {
	BaseEvent
	OrganizationID   uuid.UUID `json:"organizationId"`
	OrganizationName string    `json:"organizationName"`
	Email            string    `json:"email"`
	InviteToken      string    `json:"inviteToken"`
}

func (e OrganizationInviteCreated) EventName() string { return "identity.invite.created" }

// OrganizationCreated is published when a new organization is created.
type OrganizationCreated struct {
	BaseEvent
	OrganizationID uuid.UUID `json:"organizationId"`
	CreatedBy      uuid.UUID `json:"createdBy"`
}

func (e OrganizationCreated) EventName() string { return "identity.organization.created" }

// =============================================================================
// Partners Domain Events
// =============================================================================

// PartnerInviteCreated is published when a partner invite is created.
type PartnerInviteCreated struct {
	BaseEvent
	OrganizationID   uuid.UUID  `json:"organizationId"`
	OrganizationName string     `json:"organizationName"`
	PartnerID        uuid.UUID  `json:"partnerId"`
	PartnerName      string     `json:"partnerName"`
	Email            string     `json:"email"`
	InviteToken      string     `json:"inviteToken"`
	LeadID           *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e PartnerInviteCreated) EventName() string { return "partners.invite.created" }

// =============================================================================
// Quotes Domain Events
// =============================================================================

// QuoteSent is published when an agent sends a quote proposal to a lead via magic link.
type QuoteSent struct {
	BaseEvent
	QuoteID          uuid.UUID  `json:"quoteId"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	LeadID           uuid.UUID  `json:"leadId"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
	PublicToken      string     `json:"publicToken"`
	QuoteNumber      string     `json:"quoteNumber"`
	AgentID          uuid.UUID  `json:"agentId"`
	ConsumerEmail    string     `json:"consumerEmail"`
	ConsumerName     string     `json:"consumerName"`
	OrganizationName string     `json:"organizationName"`
}

func (e QuoteSent) EventName() string { return "quotes.quote.sent" }

// QuoteViewed is published when a lead first opens the public proposal link.
type QuoteViewed struct {
	BaseEvent
	QuoteID        uuid.UUID `json:"quoteId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	LeadID         uuid.UUID `json:"leadId"`
	ViewerIP       string    `json:"viewerIp"`
}

func (e QuoteViewed) EventName() string { return "quotes.quote.viewed" }

// QuoteUpdatedByCustomer is published when a lead toggles optional line items.
type QuoteUpdatedByCustomer struct {
	BaseEvent
	QuoteID         uuid.UUID `json:"quoteId"`
	OrganizationID  uuid.UUID `json:"organizationId"`
	ItemID          uuid.UUID `json:"itemId"`
	ItemDescription string    `json:"itemDescription"`
	IsSelected      bool      `json:"isSelected"`
	NewTotalCents   int64     `json:"newTotalCents"`
}

func (e QuoteUpdatedByCustomer) EventName() string { return "quotes.quote.updated_by_customer" }

// QuoteAnnotated is published when a customer or agent adds an annotation to a line item.
type QuoteAnnotated struct {
	BaseEvent
	QuoteID        uuid.UUID `json:"quoteId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	ItemID         uuid.UUID `json:"itemId"`
	AuthorType     string    `json:"authorType"` // "customer" or "agent"
	AuthorID       string    `json:"authorId"`
	Text           string    `json:"text"`
}

func (e QuoteAnnotated) EventName() string { return "quotes.quote.annotated" }

// QuoteAccepted is published when a lead accepts and signs the quote.
type QuoteAccepted struct {
	BaseEvent
	QuoteID          uuid.UUID  `json:"quoteId"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	LeadID           uuid.UUID  `json:"leadId"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
	SignatureName    string     `json:"signatureName"`
	TotalCents       int64      `json:"totalCents"`
	QuoteNumber      string     `json:"quoteNumber"`
	ConsumerEmail    string     `json:"consumerEmail"`
	ConsumerName     string     `json:"consumerName"`
	OrganizationName string     `json:"organizationName"`
	AgentEmail       string     `json:"agentEmail"`
	AgentName        string     `json:"agentName"`
}

func (e QuoteAccepted) EventName() string { return "quotes.quote.accepted" }

// QuoteRejected is published when a lead rejects the quote.
type QuoteRejected struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	Reason         string     `json:"reason"`
}

func (e QuoteRejected) EventName() string { return "quotes.quote.rejected" }

// =============================================================================
// Partner Offer Domain Events
// =============================================================================

// PartnerOfferCreated is published when a dispatcher generates a job offer for a vakman.
type PartnerOfferCreated struct {
	BaseEvent
	OfferID          uuid.UUID `json:"offerId"`
	OrganizationID   uuid.UUID `json:"organizationId"`
	PartnerID        uuid.UUID `json:"partnerId"`
	LeadServiceID    uuid.UUID `json:"leadServiceId"`
	LeadID           uuid.UUID `json:"leadId"`
	VakmanPriceCents int64     `json:"vakmanPriceCents"`
	PublicToken      string    `json:"publicToken"`
	PartnerName      string    `json:"partnerName"`
	PartnerPhone     string    `json:"partnerPhone"`
}

func (e PartnerOfferCreated) EventName() string { return "partners.offer.created" }

// PartnerOfferAccepted is published when a vakman accepts the job offer.
type PartnerOfferAccepted struct {
	BaseEvent
	OfferID        uuid.UUID `json:"offerId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	PartnerID      uuid.UUID `json:"partnerId"`
	LeadServiceID  uuid.UUID `json:"leadServiceId"`
	LeadID         uuid.UUID `json:"leadId"`
	PartnerName    string    `json:"partnerName"`
	PartnerEmail   string    `json:"partnerEmail"`
}

func (e PartnerOfferAccepted) EventName() string { return "partners.offer.accepted" }

// PartnerOfferRejected is published when a vakman declines the job offer.
type PartnerOfferRejected struct {
	BaseEvent
	OfferID        uuid.UUID `json:"offerId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	PartnerID      uuid.UUID `json:"partnerId"`
	LeadServiceID  uuid.UUID `json:"leadServiceId"`
	LeadID         uuid.UUID `json:"leadId"`
	PartnerName    string    `json:"partnerName"`
	Reason         string    `json:"reason,omitempty"`
}

func (e PartnerOfferRejected) EventName() string { return "partners.offer.rejected" }

// PartnerOfferExpired is published when an offer expires without a response.
type PartnerOfferExpired struct {
	BaseEvent
	OfferID        uuid.UUID `json:"offerId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	PartnerID      uuid.UUID `json:"partnerId"`
	LeadServiceID  uuid.UUID `json:"leadServiceId"`
	LeadID         uuid.UUID `json:"leadId"`
	PartnerName    string    `json:"partnerName"`
}

func (e PartnerOfferExpired) EventName() string { return "partners.offer.expired" }
