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
// This is defined by the RAC_leads domain, not by the appointments domain.
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

// AppointmentBooker is the interface that the RAC_leads domain uses to book appointments.
// The implementation is provided by the composition root (main/router) and wraps
// the appointments service. This ensures RAC_leads never directly imports the appointments domain.
type AppointmentBooker interface {
	// BookLeadVisit creates a visit appointment for a specific lead and service.
	// Returns an error if the appointment cannot be booked.
	BookLeadVisit(ctx context.Context, params BookVisitParams) error
}
