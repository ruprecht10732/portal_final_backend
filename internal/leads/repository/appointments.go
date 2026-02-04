package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// GetLeadAppointmentStats returns appointment statistics for a lead (for scoring purposes).
func (r *Repository) GetLeadAppointmentStats(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadAppointmentStats, error) {
	var stats LeadAppointmentStats

	query := `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'Scheduled') AS scheduled,
			COUNT(*) FILTER (WHERE status = 'Completed') AS completed,
			COUNT(*) FILTER (WHERE status = 'Cancelled') AS cancelled,
			EXISTS(
				SELECT 1 FROM RAC_appointments
				WHERE lead_id = $1 AND organization_id = $2
				AND status = 'Scheduled' AND start_time > NOW()
			) AS has_upcoming
		FROM RAC_appointments
		WHERE lead_id = $1 AND organization_id = $2
	`

	err := r.pool.QueryRow(ctx, query, leadID, organizationID).Scan(
		&stats.Total,
		&stats.Scheduled,
		&stats.Completed,
		&stats.Cancelled,
		&stats.HasUpcoming,
	)
	if err != nil {
		// Return zero stats on error (no RAC_appointments is valid)
		return LeadAppointmentStats{}, nil
	}

	return stats, nil
}

// GetLatestAppointment returns the most recent appointment for a lead.
func (r *Repository) GetLatestAppointment(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*time.Time, string, error) {
	var startTime time.Time
	var status string

	query := `
		SELECT start_time, status
		FROM RAC_appointments
		WHERE lead_id = $1 AND organization_id = $2
		ORDER BY start_time DESC
		LIMIT 1
	`

	err := r.pool.QueryRow(ctx, query, leadID, organizationID).Scan(&startTime, &status)
	if err != nil {
		return nil, "", nil // No appointment found
	}

	return &startTime, status, nil
}
