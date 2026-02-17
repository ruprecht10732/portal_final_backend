package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	var aggs ServiceStateAggregates

	query := `
		WITH quote_stats AS (
			SELECT
				COUNT(*) FILTER (WHERE status = 'Accepted') AS accepted,
				COUNT(*) FILTER (WHERE status = 'Sent') AS sent,
				COUNT(*) FILTER (WHERE status = 'Draft') AS draft,
				COUNT(*) FILTER (WHERE status = 'Rejected') AS rejected,
				MAX(updated_at) AS latest_at
			FROM rac_quotes
			WHERE lead_service_id = $1 AND organization_id = $2
		),
		offer_counts AS (
			SELECT
				COUNT(*) FILTER (WHERE status = 'accepted') AS accepted,
				COUNT(*) FILTER (WHERE status IN ('pending', 'sent')) AS pending
			FROM rac_partner_offers
			WHERE lead_service_id = $1 AND organization_id = $2
		),
		appt_counts AS (
			SELECT
				COUNT(*) FILTER (WHERE status IN ('scheduled', 'requested') AND start_time > NOW()) AS scheduled,
				COUNT(*) FILTER (WHERE status = 'completed') AS completed,
				COUNT(*) FILTER (WHERE status IN ('cancelled', 'no_show')) AS cancelled,
				MAX(updated_at) AS latest_at
			FROM rac_appointments
			WHERE lead_service_id = $1 AND organization_id = $2
		),
		report_check AS (
			SELECT EXISTS(
				SELECT 1
				FROM rac_appointment_visit_reports avr
				JOIN rac_appointments a ON a.id = avr.appointment_id
				WHERE a.lead_service_id = $1 AND a.organization_id = $2 AND avr.organization_id = $2
			) AS has_report
		),
		ai_data AS (
			SELECT recommended_action
			FROM rac_lead_ai_analysis
			WHERE lead_service_id = $1 AND organization_id = $2
			ORDER BY created_at DESC
			LIMIT 1
		),
		terminal_event AS (
			SELECT MAX(occurred_at) AS terminal_at
			FROM rac_lead_service_events
			WHERE lead_service_id = $1 AND organization_id = $2
				AND (
					status IN ('Completed', 'Lost', 'Disqualified')
					OR pipeline_stage IN ('Completed', 'Lost')
				)
		)
		SELECT
			COALESCE(q.accepted, 0),
			COALESCE(q.sent, 0),
			COALESCE(q.draft, 0),
			COALESCE(q.rejected, 0),
			q.latest_at,
			COALESCE(o.accepted, 0),
			COALESCE(o.pending, 0),
			COALESCE(ap.scheduled, 0),
			COALESCE(ap.completed, 0),
			COALESCE(ap.cancelled, 0),
			ap.latest_at,
			COALESCE(rc.has_report, false),
			a.recommended_action,
			t.terminal_at
		FROM quote_stats q
		CROSS JOIN offer_counts o
		CROSS JOIN appt_counts ap
		CROSS JOIN report_check rc
		LEFT JOIN ai_data a ON true
		CROSS JOIN terminal_event t
	`

	err := r.pool.QueryRow(ctx, query, serviceID, organizationID).Scan(
		&aggs.AcceptedQuotes,
		&aggs.SentQuotes,
		&aggs.DraftQuotes,
		&aggs.RejectedQuotes,
		&aggs.LatestQuoteAt,
		&aggs.AcceptedOffers,
		&aggs.PendingOffers,
		&aggs.ScheduledAppointments,
		&aggs.CompletedAppointments,
		&aggs.CancelledAppointments,
		&aggs.LatestAppointmentAt,
		&aggs.HasVisitReport,
		&aggs.AiAction,
		&aggs.TerminalAt,
	)
	if err != nil {
		return ServiceStateAggregates{}, fmt.Errorf("get service aggregates: %w", err)
	}

	return aggs, nil
}
