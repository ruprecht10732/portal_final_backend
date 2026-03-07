package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	leadsdb "portal_final_backend/internal/leads/db"
)

// GetLeadAppointmentStats returns appointment statistics for a lead (for scoring purposes).
func (r *Repository) GetLeadAppointmentStats(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadAppointmentStats, error) {
	row, err := r.queries.GetLeadAppointmentStats(ctx, leadsdb.GetLeadAppointmentStatsParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		// Return zero stats on error (no RAC_appointments is valid)
		return LeadAppointmentStats{}, nil
	}

	return LeadAppointmentStats{
		Total:       int(row.Total),
		Scheduled:   int(row.Scheduled),
		Completed:   int(row.Completed),
		Cancelled:   int(row.Cancelled),
		HasUpcoming: row.HasUpcoming,
	}, nil
}

// GetLatestAppointment returns the most recent appointment for a lead.
func (r *Repository) GetLatestAppointment(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*time.Time, string, error) {
	row, err := r.queries.GetLatestAppointment(ctx, leadsdb.GetLatestAppointmentParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil // No appointment found
	}
	if err != nil {
		return nil, "", err
	}

	return optionalTime(row.StartTime), row.Status, nil
}
