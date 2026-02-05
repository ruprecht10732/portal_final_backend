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

// BookVisitParams contains the parameters needed to book a lead visit appointment.
// This is defined by the RAC_leads domain, not by the RAC_appointments domain.
type BookVisitParams struct {
	TenantID              uuid.UUID
	UserID                uuid.UUID // The agent booking the visit (and likely attending)
	LeadID                uuid.UUID
	LeadServiceID         uuid.UUID
	StartTime             time.Time
	EndTime               time.Time
	Title                 string
	Description           string
	SendConfirmationEmail bool // If true, sends confirmation email to lead
}

// LeadVisitSummary represents the minimal lead visit details needed by RAC_leads.
type LeadVisitSummary struct {
	AppointmentID uuid.UUID
	UserID        uuid.UUID
	StartTime     time.Time
	EndTime       time.Time
}

// RescheduleVisitParams contains the parameters needed to reschedule a lead visit.
type RescheduleVisitParams struct {
	TenantID      uuid.UUID
	UserID        uuid.UUID
	LeadServiceID uuid.UUID
	StartTime     time.Time
	EndTime       time.Time
	Title         *string
	Description   *string
}

// CancelVisitParams contains the parameters needed to cancel a lead visit.
type CancelVisitParams struct {
	TenantID      uuid.UUID
	UserID        uuid.UUID
	LeadServiceID uuid.UUID
}

// AppointmentBooker is the interface that the RAC_leads domain uses to book RAC_appointments.
// The implementation is provided by the composition root (main/router) and wraps
// the RAC_appointments service. This ensures RAC_leads never directly imports the RAC_appointments domain.
type AppointmentBooker interface {
	// BookLeadVisit creates a visit appointment for a specific lead and service.
	// Returns an error if the appointment cannot be booked.
	BookLeadVisit(ctx context.Context, params BookVisitParams) error
	// GetLeadVisitByService retrieves the latest non-cancelled appointment for a lead service.
	GetLeadVisitByService(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID, userID uuid.UUID) (*LeadVisitSummary, error)
	// RescheduleLeadVisit updates the time (and optionally title/description) of a lead visit.
	RescheduleLeadVisit(ctx context.Context, params RescheduleVisitParams) error
	// CancelLeadVisit cancels the lead visit appointment for a lead service.
	CancelLeadVisit(ctx context.Context, params CancelVisitParams) error
}
