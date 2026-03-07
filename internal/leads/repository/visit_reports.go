package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AppointmentVisitReport struct {
	AppointmentID    uuid.UUID
	OrganizationID   uuid.UUID
	Measurements     *string
	AccessDifficulty *string
	Notes            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (r *Repository) GetAppointmentVisitReport(ctx context.Context, appointmentID uuid.UUID, organizationID uuid.UUID) (*AppointmentVisitReport, error) {
	var report AppointmentVisitReport
	query := `
		SELECT appointment_id, organization_id, measurements, access_difficulty, notes, created_at, updated_at
		FROM RAC_appointment_visit_reports
		WHERE appointment_id = $1 AND organization_id = $2
	`

	err := r.pool.QueryRow(ctx, query, appointmentID, organizationID).Scan(
		&report.AppointmentID,
		&report.OrganizationID,
		&report.Measurements,
		&report.AccessDifficulty,
		&report.Notes,
		&report.CreatedAt,
		&report.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment visit report: %w", err)
	}
	return &report, nil
}

func (r *Repository) GetLatestAppointmentVisitReportByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (*AppointmentVisitReport, error) {
	var report AppointmentVisitReport
	query := `
		SELECT avr.appointment_id, avr.organization_id, avr.measurements, avr.access_difficulty, avr.notes, avr.created_at, avr.updated_at
		FROM RAC_appointment_visit_reports avr
		JOIN RAC_appointments a ON a.id = avr.appointment_id AND a.organization_id = avr.organization_id
		WHERE a.lead_service_id = $1 AND avr.organization_id = $2 AND a.status != 'cancelled'
		ORDER BY a.start_time DESC, avr.updated_at DESC
		LIMIT 1
	`

	err := r.pool.QueryRow(ctx, query, serviceID, organizationID).Scan(
		&report.AppointmentID,
		&report.OrganizationID,
		&report.Measurements,
		&report.AccessDifficulty,
		&report.Notes,
		&report.CreatedAt,
		&report.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest appointment visit report by service: %w", err)
	}
	return &report, nil
}
