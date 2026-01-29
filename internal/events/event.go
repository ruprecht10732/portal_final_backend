// Package events provides domain event definitions and an event bus for
// decoupled, event-driven communication between modules.
package events

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is the base interface all domain events must implement.
type Event interface {
	// EventName returns a unique identifier for the event type.
	EventName() string
	// OccurredAt returns when the event occurred.
	OccurredAt() time.Time
}

// BaseEvent provides common fields for all events.
type BaseEvent struct {
	Timestamp time.Time `json:"timestamp"`
}

func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

func NewBaseEvent() BaseEvent {
	return BaseEvent{Timestamp: time.Now()}
}

// Handler processes events of a specific type.
type Handler interface {
	Handle(ctx context.Context, event Event) error
}

// HandlerFunc is an adapter to allow ordinary functions to be used as handlers.
type HandlerFunc func(ctx context.Context, event Event) error

func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Bus is the interface for publishing and subscribing to domain events.
type Bus interface {
	// Publish sends an event to all registered handlers for that event type.
	// Handlers are executed asynchronously by default.
	Publish(ctx context.Context, event Event)

	// PublishSync sends an event and waits for all handlers to complete.
	PublishSync(ctx context.Context, event Event) error

	// Subscribe registers a handler for a specific event type.
	// The eventName should match the value returned by Event.EventName().
	Subscribe(eventName string, handler Handler)
}

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
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
	ServiceType     string     `json:"serviceType"`
}

func (e LeadCreated) EventName() string { return "leads.lead.created" }

// LeadAssigned is published when a lead is assigned to an agent.
type LeadAssigned struct {
	BaseEvent
	LeadID        uuid.UUID  `json:"leadId"`
	PreviousAgent *uuid.UUID `json:"previousAgent,omitempty"`
	NewAgent      *uuid.UUID `json:"newAgent,omitempty"`
	AssignedByID  uuid.UUID  `json:"assignedById"`
}

func (e LeadAssigned) EventName() string { return "leads.lead.assigned" }

// VisitScheduled is published when a visit is scheduled for a lead.
type VisitScheduled struct {
	BaseEvent
	LeadID        uuid.UUID  `json:"leadId"`
	ServiceID     uuid.UUID  `json:"serviceId"`
	ScheduledDate time.Time  `json:"scheduledDate"`
	ScoutID       *uuid.UUID `json:"scoutId,omitempty"`
	// Consumer details for notification
	ConsumerEmail     *string `json:"consumerEmail,omitempty"`
	ConsumerFirstName string  `json:"consumerFirstName"`
	ConsumerLastName  string  `json:"consumerLastName"`
	// Address for notification
	AddressStreet      string `json:"addressStreet"`
	AddressHouseNumber string `json:"addressHouseNumber"`
	AddressZipCode     string `json:"addressZipCode"`
	AddressCity        string `json:"addressCity"`
	// Whether to send invite
	SendInvite bool `json:"sendInvite"`
}

func (e VisitScheduled) EventName() string { return "leads.visit.scheduled" }

// VisitRescheduled is published when a visit is rescheduled.
type VisitRescheduled struct {
	BaseEvent
	LeadID           uuid.UUID  `json:"leadId"`
	ServiceID        uuid.UUID  `json:"serviceId"`
	PreviousDate     *time.Time `json:"previousDate,omitempty"`
	NewScheduledDate time.Time  `json:"newScheduledDate"`
	ScoutID          *uuid.UUID `json:"scoutId,omitempty"`
	MarkedAsNoShow   bool       `json:"markedAsNoShow"`
	// Consumer details for notification
	ConsumerEmail     *string `json:"consumerEmail,omitempty"`
	ConsumerFirstName string  `json:"consumerFirstName"`
	ConsumerLastName  string  `json:"consumerLastName"`
	// Address for notification
	AddressStreet      string `json:"addressStreet"`
	AddressHouseNumber string `json:"addressHouseNumber"`
	AddressZipCode     string `json:"addressZipCode"`
	AddressCity        string `json:"addressCity"`
	// Whether to send invite
	SendInvite bool `json:"sendInvite"`
}

func (e VisitRescheduled) EventName() string { return "leads.visit.rescheduled" }

// SurveyCompleted is published when a survey/visit is completed.
type SurveyCompleted struct {
	BaseEvent
	LeadID           uuid.UUID `json:"leadId"`
	ServiceID        uuid.UUID `json:"serviceId"`
	Measurements     string    `json:"measurements"`
	AccessDifficulty string    `json:"accessDifficulty"`
}

func (e SurveyCompleted) EventName() string { return "leads.survey.completed" }

// LeadMarkedNoShow is published when a lead is marked as no-show.
type LeadMarkedNoShow struct {
	BaseEvent
	LeadID    uuid.UUID `json:"leadId"`
	ServiceID uuid.UUID `json:"serviceId"`
	Notes     string    `json:"notes"`
}

func (e LeadMarkedNoShow) EventName() string { return "leads.visit.no_show" }
