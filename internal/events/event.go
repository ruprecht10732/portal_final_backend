// Package events provides domain event definitions for decoupled,
// event-driven communication between modules.
// Infrastructure (Bus, Handler) is in platform/events.
package events

import (
	"time"

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
	LeadServiceID   uuid.UUID  `json:"leadServiceId"`
	TenantID        uuid.UUID  `json:"tenantId"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
	ServiceType     string     `json:"serviceType"`
	Source          string     `json:"source,omitempty"`
	ConsumerName    string     `json:"consumerName"`
	ConsumerPhone   string     `json:"consumerPhone"`
	ConsumerEmail   string     `json:"consumerEmail"`
	WhatsAppOptedIn bool       `json:"whatsappOptedIn"`
	PublicToken     string     `json:"publicToken"`
}

func (e LeadCreated) EventName() string { return "RAC_leads.lead.created" }

// LeadAssigned is published when a lead is assigned to an agent.
type LeadAssigned struct {
	BaseEvent
	LeadID        uuid.UUID  `json:"leadId"`
	TenantID      uuid.UUID  `json:"tenantId"`
	PreviousAgent *uuid.UUID `json:"previousAgent,omitempty"`
	NewAgent      *uuid.UUID `json:"newAgent,omitempty"`
	AssignedByID  uuid.UUID  `json:"assignedById"`
}

func (e LeadAssigned) EventName() string { return "leads.assigned" }

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

// AttachmentUploaded is published when a lead service attachment is created.
type AttachmentUploaded struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	AttachmentID  uuid.UUID `json:"attachmentId"`
	FileName      string    `json:"fileName"`
	FileKey       string    `json:"fileKey"`
	ContentType   string    `json:"contentType"`
	SizeBytes     int64     `json:"sizeBytes"`
}

func (e AttachmentUploaded) EventName() string { return "leads.attachment.uploaded" }

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

// LeadAutoDisqualified is published when the system automatically disqualifies a lead service.
// Used for transparency and downstream handlers (e.g. notifications).
type LeadAutoDisqualified struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Reason        string    `json:"reason"` // e.g. "junk_quality"
}

func (e LeadAutoDisqualified) EventName() string { return "leads.lead.auto_disqualified" }

// LeadServiceStatusChanged is published when a user manually updates the status
// of a lead service. This triggers state reconciliation so the pipeline stage
// stays consistent with the new status.
type LeadServiceStatusChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	OldStatus     string    `json:"oldStatus"`
	NewStatus     string    `json:"newStatus"`
}

func (e LeadServiceStatusChanged) EventName() string { return "leads.service.status_changed" }

// =============================================================================
// Webhook Domain Events
// =============================================================================

// WebhookLeadCreated is published when a lead is created via the webhook form capture.
type WebhookLeadCreated struct {
	BaseEvent
	LeadID       uuid.UUID `json:"leadId"`
	TenantID     uuid.UUID `json:"tenantId"`
	SourceDomain string    `json:"sourceDomain"`
	IsIncomplete bool      `json:"isIncomplete"`
}

func (e WebhookLeadCreated) EventName() string { return "webhook.lead.created" }

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
	ConsumerPhone    string     `json:"consumerPhone"`
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

// QuoteCreated is published when a quote is created.
// This event exists to trigger state reconciliation (e.g. resurrecting a Lost service).
type QuoteCreated struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	QuoteNumber    string     `json:"quoteNumber"`
	ActorID        *uuid.UUID `json:"actorId,omitempty"`
}

func (e QuoteCreated) EventName() string { return "quotes.quote.created" }

// QuoteDeleted is published when a quote is deleted.
// This event exists to trigger state reconciliation (e.g. reverting Estimation stage when last draft is removed).
type QuoteDeleted struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	QuoteNumber    string     `json:"quoteNumber"`
	ActorID        *uuid.UUID `json:"actorId,omitempty"`
}

func (e QuoteDeleted) EventName() string { return "quotes.quote.deleted" }

// QuoteStatusChanged is published when a quote status is updated via the admin API.
// This triggers state reconciliation so the pipeline reflects the new status.
type QuoteStatusChanged struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	QuoteNumber    string     `json:"quoteNumber"`
	OldStatus      string     `json:"oldStatus"`
	NewStatus      string     `json:"newStatus"`
	ActorID        uuid.UUID  `json:"actorId"`
}

func (e QuoteStatusChanged) EventName() string { return "quotes.quote.status_changed" }

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
	ConsumerPhone    string     `json:"consumerPhone"`
	OrganizationName string     `json:"organizationName"`
	AgentEmail       string     `json:"agentEmail"`
	AgentName        string     `json:"agentName"`
	PublicToken      string     `json:"publicToken"`
}

func (e QuoteAccepted) EventName() string { return "quotes.quote.accepted" }

// QuoteRejected is published when a lead rejects the quote.
type QuoteRejected struct {
	BaseEvent
	QuoteID          uuid.UUID  `json:"quoteId"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	LeadID           uuid.UUID  `json:"leadId"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
	Reason           string     `json:"reason"`
	ConsumerEmail    string     `json:"consumerEmail"`
	ConsumerName     string     `json:"consumerName"`
	ConsumerPhone    string     `json:"consumerPhone"`
	OrganizationName string     `json:"organizationName"`
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
	PartnerEmail     string    `json:"partnerEmail"`
}

func (e PartnerOfferCreated) EventName() string { return "partners.offer.created" }

// PartnerOfferAccepted is published when a vakman accepts the job offer.
type PartnerOfferAccepted struct {
	BaseEvent
	OfferID                uuid.UUID `json:"offerId"`
	OrganizationID         uuid.UUID `json:"organizationId"`
	PartnerID              uuid.UUID `json:"partnerId"`
	LeadServiceID          uuid.UUID `json:"leadServiceId"`
	LeadID                 uuid.UUID `json:"leadId"`
	PartnerName            string    `json:"partnerName"`
	PartnerEmail           string    `json:"partnerEmail"`
	PartnerPhone           string    `json:"partnerPhone"`
	PartnerWhatsAppOptedIn bool      `json:"partnerWhatsappOptedIn"`
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

// PartnerOfferDeleted is published when a pending/sent/expired offer is deleted.
// This triggers state reconciliation so the pipeline regresses correctly.
type PartnerOfferDeleted struct {
	BaseEvent
	OfferID        uuid.UUID `json:"offerId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	PartnerID      uuid.UUID `json:"partnerId"`
	LeadServiceID  uuid.UUID `json:"leadServiceId"`
	LeadID         uuid.UUID `json:"leadId"`
}

func (e PartnerOfferDeleted) EventName() string { return "partners.offer.deleted" }

// =============================================================================
// Appointments Domain Events
// =============================================================================

// AppointmentCreated is published when an appointment is scheduled.
type AppointmentCreated struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	StartTime      time.Time  `json:"startTime"`
	EndTime        time.Time  `json:"endTime"`
	ConsumerName   string     `json:"consumerName,omitempty"`
	ConsumerPhone  string     `json:"consumerPhone,omitempty"`
	ConsumerEmail  string     `json:"consumerEmail,omitempty"`
	Location       string     `json:"location,omitempty"`
}

func (e AppointmentCreated) EventName() string { return "appointments.created" }

// AppointmentStatusChanged is published when an appointment's status changes (e.g. cancelled, completed).
// This event exists to trigger service state reconciliation.
type AppointmentStatusChanged struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
	OldStatus      string     `json:"oldStatus"`
	NewStatus      string     `json:"newStatus"`
}

func (e AppointmentStatusChanged) EventName() string { return "appointments.status.changed" }

// AppointmentDeleted is published when an appointment is deleted.
// This triggers state reconciliation so the pipeline regresses if needed.
type AppointmentDeleted struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
}

func (e AppointmentDeleted) EventName() string { return "appointments.deleted" }

// AppointmentReminderDue is published when a reminder should be sent.
type AppointmentReminderDue struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	StartTime      time.Time  `json:"startTime"`
	EndTime        time.Time  `json:"endTime"`
	ConsumerName   string     `json:"consumerName,omitempty"`
	ConsumerPhone  string     `json:"consumerPhone,omitempty"`
	ConsumerEmail  string     `json:"consumerEmail,omitempty"`
	Location       string     `json:"location,omitempty"`
}

func (e AppointmentReminderDue) EventName() string { return "appointments.reminder.due" }

// =============================================================================
// Notification Domain Events
// =============================================================================

// NotificationOutboxDue is published by the scheduler when a notification outbox
// record should be processed.
type NotificationOutboxDue struct {
	BaseEvent
	OutboxID uuid.UUID `json:"outboxId"`
	TenantID uuid.UUID `json:"tenantId"`
}

func (e NotificationOutboxDue) EventName() string { return "notification.outbox.due" }

// =============================================================================
// AI Analysis Domain Events
// =============================================================================

// PhotoAnalysisCompleted is published when the AI finishes analyzing lead photos.
type PhotoAnalysisCompleted struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	PhotoCount    int       `json:"photoCount"`
	Summary       string    `json:"summary"`
}

func (e PhotoAnalysisCompleted) EventName() string { return "ai.photo_analysis.completed" }

// PhotoAnalysisFailed is published when photo analysis cannot be completed.
type PhotoAnalysisFailed struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	ErrorCode     string    `json:"errorCode"`
	ErrorMessage  string    `json:"errorMessage"`
}

func (e PhotoAnalysisFailed) EventName() string { return "ai.photo_analysis.failed" }
