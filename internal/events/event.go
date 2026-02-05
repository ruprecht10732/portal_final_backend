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
