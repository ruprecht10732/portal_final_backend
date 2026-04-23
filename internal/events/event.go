// Package events provides domain event definitions for decoupled,
// event-driven communication between modules.
package events

import (
	"time"

	"portal_final_backend/platform/events"

	"github.com/google/uuid"
)

// --- Platform Re-exports ---
type (
	Event       = events.Event
	Bus         = events.Bus
	Handler     = events.Handler
	HandlerFunc = events.HandlerFunc
	BaseEvent   = events.BaseEvent
)

var NewBaseEvent = events.NewBaseEvent

// ─── Auth Domain Events ──────────────────────────────────────────────────────

type UserSignedUp struct {
	BaseEvent
	UserID      uuid.UUID `json:"userId"`
	Email       string    `json:"email"`
	VerifyToken string    `json:"verifyToken"`
}

func (e UserSignedUp) EventName() string { return "auth.user.signed_up" }

type EmailVerificationRequested struct {
	BaseEvent
	UserID      uuid.UUID `json:"userId"`
	Email       string    `json:"email"`
	VerifyToken string    `json:"verifyToken"`
}

func (e EmailVerificationRequested) EventName() string { return "auth.email.verification_requested" }

type PasswordResetRequested struct {
	BaseEvent
	UserID     uuid.UUID `json:"userId"`
	Email      string    `json:"email"`
	ResetToken string    `json:"resetToken"`
}

func (e PasswordResetRequested) EventName() string { return "auth.password.reset_requested" }

// ─── Leads Domain Events ─────────────────────────────────────────────────────

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
	PublicToken     string     `json:"publicToken"`
	WhatsAppOptedIn bool       `json:"whatsappOptedIn"`
}

func (e LeadCreated) EventName() string { return "leads.lead.created" }

type LeadAssigned struct {
	BaseEvent
	LeadID        uuid.UUID  `json:"leadId"`
	TenantID      uuid.UUID  `json:"tenantId"`
	AssignedByID  uuid.UUID  `json:"assignedById"`
	PreviousAgent *uuid.UUID `json:"previousAgent,omitempty"`
	NewAgent      *uuid.UUID `json:"newAgent,omitempty"`
}

func (e LeadAssigned) EventName() string { return "leads.lead.assigned" }

type LeadServiceAdded struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	ServiceType   string    `json:"serviceType"`
}

func (e LeadServiceAdded) EventName() string { return "leads.service.added" }

type LeadDataChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Source        string    `json:"source"`
}

func (e LeadDataChanged) EventName() string { return "leads.data.changed" }

type LeadAutoDisqualified struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Reason        string    `json:"reason"`
}

func (e LeadAutoDisqualified) EventName() string { return "leads.lead.auto_disqualified" }

type LeadServiceStatusChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	OldStatus     string    `json:"oldStatus"`
	NewStatus     string    `json:"newStatus"`
}

func (e LeadServiceStatusChanged) EventName() string { return "leads.service.status_changed" }

type PipelineStageChanged struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	OldStage      string    `json:"oldStage"`
	NewStage      string    `json:"newStage"`
	Reason        string    `json:"reason,omitempty"`
	ReasonCode    string    `json:"reasonCode,omitempty"`
	Trigger       string    `json:"trigger,omitempty"`
	ActorType     string    `json:"actorType,omitempty"`
	ActorName     string    `json:"actorName,omitempty"`
	RunID         string    `json:"runId,omitempty"`
}

func (e PipelineStageChanged) EventName() string { return "leads.pipeline.stage_changed" }

type ManualInterventionRequired struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Reason        string    `json:"reason"`
	ReasonCode    string    `json:"reasonCode,omitempty"`
	Context       string    `json:"context,omitempty"`
	RunID         string    `json:"runId,omitempty"`
}

func (e ManualInterventionRequired) EventName() string { return "leads.intervention.required" }

type AuditCompleted struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	Findings      []string  `json:"findings,omitempty"`
	Passed        bool      `json:"passed"`
}

func (e AuditCompleted) EventName() string { return "leads.audit.completed" }

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

// ─── Webhook Domain Events ───────────────────────────────────────────────────

type WebhookLeadCreated struct {
	BaseEvent
	LeadID       uuid.UUID `json:"leadId"`
	TenantID     uuid.UUID `json:"tenantId"`
	SourceDomain string    `json:"sourceDomain"`
	IsIncomplete bool      `json:"isIncomplete"`
}

func (e WebhookLeadCreated) EventName() string { return "webhooks.lead.created" }

// ─── Identity Domain Events ──────────────────────────────────────────────────

type OrganizationCreated struct {
	BaseEvent
	OrganizationID uuid.UUID `json:"organizationId"`
	CreatedBy      uuid.UUID `json:"createdBy"`
}

func (e OrganizationCreated) EventName() string { return "identity.organization.created" }

type OrganizationInviteCreated struct {
	BaseEvent
	OrganizationID   uuid.UUID `json:"organizationId"`
	OrganizationName string    `json:"organizationName"`
	Email            string    `json:"email"`
	InviteToken      string    `json:"inviteToken"`
}

func (e OrganizationInviteCreated) EventName() string { return "identity.invite.created" }

// ─── Partners Domain Events ──────────────────────────────────────────────────

type PartnerInviteCreated struct {
	BaseEvent
	OrganizationID   uuid.UUID  `json:"organizationId"`
	PartnerID        uuid.UUID  `json:"partnerId"`
	OrganizationName string     `json:"organizationName"`
	PartnerName      string     `json:"partnerName"`
	Email            string     `json:"email"`
	InviteToken      string     `json:"inviteToken"`
	LeadID           *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e PartnerInviteCreated) EventName() string { return "partners.invite.created" }

type PartnerOfferCreated struct {
	BaseEvent
	OfferID          uuid.UUID `json:"offerId"`
	OrganizationID   uuid.UUID `json:"organizationId"`
	PartnerID        uuid.UUID `json:"partnerId"`
	LeadServiceID    uuid.UUID `json:"leadServiceId"`
	LeadID           uuid.UUID `json:"leadId"`
	OrganizationName string    `json:"organizationName"`
	PartnerName      string    `json:"partnerName"`
	PartnerPhone     string    `json:"partnerPhone"`
	PartnerEmail     string    `json:"partnerEmail"`
	PublicToken      string    `json:"publicToken"`
	VakmanPriceCents int64     `json:"vakmanPriceCents"`
}

func (e PartnerOfferCreated) EventName() string { return "partners.offer.created" }

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

type PartnerOfferDeleted struct {
	BaseEvent
	OfferID        uuid.UUID `json:"offerId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	PartnerID      uuid.UUID `json:"partnerId"`
	LeadServiceID  uuid.UUID `json:"leadServiceId"`
	LeadID         uuid.UUID `json:"leadId"`
}

func (e PartnerOfferDeleted) EventName() string { return "partners.offer.deleted" }

// ─── Quotes Domain Events ────────────────────────────────────────────────────

type QuoteCreated struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	QuoteNumber    string     `json:"quoteNumber"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	ActorID        *uuid.UUID `json:"actorId,omitempty"`
}

func (e QuoteCreated) EventName() string { return "quotes.quote.created" }

type QuoteSent struct {
	BaseEvent
	QuoteID          uuid.UUID      `json:"quoteId"`
	OrganizationID   uuid.UUID      `json:"organizationId"`
	LeadID           uuid.UUID      `json:"leadId"`
	AgentID          uuid.UUID      `json:"agentId"`
	QuoteNumber      string         `json:"quoteNumber"`
	PublicToken      string         `json:"publicToken"`
	ConsumerEmail    string         `json:"consumerEmail"`
	ConsumerName     string         `json:"consumerName"`
	ConsumerPhone    string         `json:"consumerPhone"`
	OrganizationName string         `json:"organizationName"`
	LeadServiceID    *uuid.UUID     `json:"leadServiceId,omitempty"`
	ISDESubsidy      map[string]any `json:"isdeSubsidy,omitempty"`
}

func (e QuoteSent) EventName() string { return "quotes.quote.sent" }

type QuoteViewed struct {
	BaseEvent
	QuoteID        uuid.UUID `json:"quoteId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	LeadID         uuid.UUID `json:"leadId"`
	QuoteNumber    string    `json:"quoteNumber"`
	ViewerIP       string    `json:"viewerIp"`
}

func (e QuoteViewed) EventName() string { return "quotes.quote.viewed" }

type QuoteUpdatedByCustomer struct {
	BaseEvent
	QuoteID         uuid.UUID `json:"quoteId"`
	OrganizationID  uuid.UUID `json:"organizationId"`
	ItemID          uuid.UUID `json:"itemId"`
	ItemDescription string    `json:"itemDescription"`
	NewTotalCents   int64     `json:"newTotalCents"`
	IsSelected      bool      `json:"isSelected"`
}

func (e QuoteUpdatedByCustomer) EventName() string { return "quotes.quote.updated_by_customer" }

type QuoteAnnotated struct {
	BaseEvent
	QuoteID          uuid.UUID  `json:"quoteId"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	LeadID           uuid.UUID  `json:"leadId"`
	ItemID           uuid.UUID  `json:"itemId"`
	QuoteNumber      string     `json:"quoteNumber"`
	PublicToken      string     `json:"publicToken"`
	ItemDescription  string     `json:"itemDescription"`
	AuthorType       string     `json:"authorType"` // "customer" or "agent"
	AuthorID         string     `json:"authorId"`
	Text             string     `json:"text"`
	ConsumerEmail    string     `json:"consumerEmail"`
	ConsumerName     string     `json:"consumerName"`
	ConsumerPhone    string     `json:"consumerPhone"`
	OrganizationName string     `json:"organizationName"`
	CreatorEmail     string     `json:"creatorEmail"`
	CreatorName      string     `json:"creatorName"`
	CreatorPhone     string     `json:"creatorPhone"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
	CreatorID        *uuid.UUID `json:"creatorId,omitempty"`
}

func (e QuoteAnnotated) EventName() string { return "quotes.quote.annotated" }

type QuoteAccepted struct {
	BaseEvent
	QuoteID          uuid.UUID      `json:"quoteId"`
	OrganizationID   uuid.UUID      `json:"organizationId"`
	LeadID           uuid.UUID      `json:"leadId"`
	QuoteNumber      string         `json:"quoteNumber"`
	SignatureName    string         `json:"signatureName"`
	ConsumerEmail    string         `json:"consumerEmail"`
	ConsumerName     string         `json:"consumerName"`
	ConsumerPhone    string         `json:"consumerPhone"`
	OrganizationName string         `json:"organizationName"`
	AgentEmail       string         `json:"agentEmail"`
	AgentName        string         `json:"agentName"`
	PublicToken      string         `json:"publicToken"`
	LeadServiceID    *uuid.UUID     `json:"leadServiceId,omitempty"`
	ISDESubsidy      map[string]any `json:"isdeSubsidy,omitempty"`
	TotalCents       int64          `json:"totalCents"`
}

func (e QuoteAccepted) EventName() string { return "quotes.quote.accepted" }

type QuoteRejected struct {
	BaseEvent
	QuoteID          uuid.UUID  `json:"quoteId"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	LeadID           uuid.UUID  `json:"leadId"`
	QuoteNumber      string     `json:"quoteNumber"`
	Reason           string     `json:"reason"`
	ConsumerEmail    string     `json:"consumerEmail"`
	ConsumerName     string     `json:"consumerName"`
	ConsumerPhone    string     `json:"consumerPhone"`
	OrganizationName string     `json:"organizationName"`
	LeadServiceID    *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e QuoteRejected) EventName() string { return "quotes.quote.rejected" }

type QuoteStatusChanged struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	ActorID        uuid.UUID  `json:"actorId"`
	QuoteNumber    string     `json:"quoteNumber"`
	OldStatus      string     `json:"oldStatus"`
	NewStatus      string     `json:"newStatus"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e QuoteStatusChanged) EventName() string { return "quotes.quote.status_changed" }

type QuoteDeleted struct {
	BaseEvent
	QuoteID        uuid.UUID  `json:"quoteId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	LeadID         uuid.UUID  `json:"leadId"`
	QuoteNumber    string     `json:"quoteNumber"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
	ActorID        *uuid.UUID `json:"actorId,omitempty"`
}

func (e QuoteDeleted) EventName() string { return "quotes.quote.deleted" }

// ─── Appointments Domain Events ──────────────────────────────────────────────

type AppointmentCreated struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	UserID         uuid.UUID  `json:"userId"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	StartTime      time.Time  `json:"startTime"`
	EndTime        time.Time  `json:"endTime"`
	ConsumerName   string     `json:"consumerName,omitempty"`
	ConsumerPhone  string     `json:"consumerPhone,omitempty"`
	ConsumerEmail  string     `json:"consumerEmail,omitempty"`
	Location       string     `json:"location,omitempty"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e AppointmentCreated) EventName() string { return "appointments.appointment.created" }

type AppointmentStatusChanged struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	UserID         uuid.UUID  `json:"userId"`
	OldStatus      string     `json:"oldStatus"`
	NewStatus      string     `json:"newStatus"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e AppointmentStatusChanged) EventName() string {
	return "appointments.appointment.status_changed"
}

type AppointmentDeleted struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	UserID         uuid.UUID  `json:"userId"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e AppointmentDeleted) EventName() string { return "appointments.appointment.deleted" }

type VisitReportSubmitted struct {
	BaseEvent
	AppointmentID uuid.UUID `json:"appointmentId"`
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
}

func (e VisitReportSubmitted) EventName() string { return "appointments.visit_report.submitted" }

type AppointmentReminderDue struct {
	BaseEvent
	AppointmentID  uuid.UUID  `json:"appointmentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	UserID         uuid.UUID  `json:"userId"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	StartTime      time.Time  `json:"startTime"`
	EndTime        time.Time  `json:"endTime"`
	ConsumerName   string     `json:"consumerName,omitempty"`
	ConsumerPhone  string     `json:"consumerPhone,omitempty"`
	ConsumerEmail  string     `json:"consumerEmail,omitempty"`
	Location       string     `json:"location,omitempty"`
	LeadID         *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID  *uuid.UUID `json:"leadServiceId,omitempty"`
}

func (e AppointmentReminderDue) EventName() string { return "appointments.appointment.reminder_due" }

// ─── AI Analysis Domain Events ───────────────────────────────────────────────

type PhotoAnalysisCompleted struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	PhotoCount    int       `json:"photoCount"`
	Summary       string    `json:"summary"`
}

func (e PhotoAnalysisCompleted) EventName() string { return "ai.photo_analysis.completed" }

type PhotoAnalysisFailed struct {
	BaseEvent
	LeadID        uuid.UUID `json:"leadId"`
	LeadServiceID uuid.UUID `json:"leadServiceId"`
	TenantID      uuid.UUID `json:"tenantId"`
	ErrorCode     string    `json:"errorCode"`
	ErrorMessage  string    `json:"errorMessage"`
}

func (e PhotoAnalysisFailed) EventName() string { return "ai.photo_analysis.failed" }

// ─── Infrastructure Domain Events ────────────────────────────────────────────

type NewEmailReceived struct {
	BaseEvent
	AccountID   uuid.UUID `json:"accountId"`
	UserID      uuid.UUID `json:"userId"`
	MessageID   string    `json:"messageId"`
	FromAddress string    `json:"fromAddress"`
	Subject     string    `json:"subject"`
	UID         int64     `json:"uid"`
}

func (e NewEmailReceived) EventName() string { return "imap.email.received" }

type NotificationOutboxDue struct {
	BaseEvent
	OutboxID uuid.UUID `json:"outboxId"`
	TenantID uuid.UUID `json:"tenantId"`
}

func (e NotificationOutboxDue) EventName() string { return "notifications.outbox.due" }
