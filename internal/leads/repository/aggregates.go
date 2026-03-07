package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	leadsdb "portal_final_backend/internal/leads/db"
)

// ServiceStateAggregates holds counts and timestamps to determine service state contextually.
// It is used as the reconciliation "source of truth" for deriving pipeline stage/status.
//
// Notes:
// - Use unquoted lowercase rac_* table names. Postgres folds unquoted identifiers to lowercase.
// - Quotes use quote_status enum values with capitalized strings: Draft/Sent/Accepted/Rejected/Expired.
// - Appointments use lowercase text statuses: scheduled/requested/completed/cancelled/no_show.
// - Partner offers use offer_status enum values: pending/sent/accepted/rejected/expired.
type ServiceStateAggregates struct {
	// Quotes
	AcceptedQuotes int
	SentQuotes     int
	DraftQuotes    int
	RejectedQuotes int
	LatestQuoteAt  *time.Time

	// Offers
	AcceptedOffers int
	PendingOffers  int
	LatestOfferAt  *time.Time

	// Appointments
	ScheduledAppointments int
	CompletedAppointments int
	CancelledAppointments int
	LatestAppointmentAt   *time.Time

	// Visit Reports
	HasVisitReport bool

	// AI
	AiAction *string

	// LeadService
	// TerminalAt is the most recent timestamp where the lead service was in a terminal state
	// (by status or pipeline stage), based on rac_lead_service_events.
	TerminalAt *time.Time
}

// GetServiceStateAggregates returns rich data for deep reconciliation.
func (r *Repository) GetServiceStateAggregates(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (ServiceStateAggregates, error) {
	row, err := r.queries.GetServiceStateAggregates(ctx, leadsdb.GetServiceStateAggregatesParams{
		LeadServiceID:  toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return ServiceStateAggregates{}, fmt.Errorf("get service aggregates: %w", err)
	}

	return ServiceStateAggregates{
		AcceptedQuotes:        int(row.Accepted),
		SentQuotes:            int(row.Sent),
		DraftQuotes:           int(row.Draft),
		RejectedQuotes:        int(row.Rejected),
		LatestQuoteAt:         optionalTimeValue(row.LatestAt),
		AcceptedOffers:        int(row.Accepted_2),
		PendingOffers:         int(row.Pending),
		LatestOfferAt:         optionalTimeValue(row.LatestAt_2),
		ScheduledAppointments: int(row.Scheduled),
		CompletedAppointments: int(row.Completed),
		CancelledAppointments: int(row.Cancelled),
		LatestAppointmentAt:   optionalTimeValue(row.LatestAt_3),
		HasVisitReport:        row.HasReport,
		AiAction:              optionalString(row.RecommendedAction),
		TerminalAt:            optionalTimeValue(row.TerminalAt),
	}, nil
}
