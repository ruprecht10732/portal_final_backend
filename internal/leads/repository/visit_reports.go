package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
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
	row, err := r.queries.GetAppointmentVisitReport(ctx, leadsdb.GetAppointmentVisitReportParams{
		AppointmentID:  toPgUUID(appointmentID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get appointment visit report: %w", err)
	}
	return appointmentVisitReportFromRow(row.AppointmentID, row.OrganizationID, row.Measurements, row.AccessDifficulty, row.Notes, row.CreatedAt, row.UpdatedAt), nil
}

func (r *Repository) GetLatestAppointmentVisitReportByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (*AppointmentVisitReport, error) {
	row, err := r.queries.GetLatestAppointmentVisitReportByService(ctx, leadsdb.GetLatestAppointmentVisitReportByServiceParams{
		LeadServiceID:  toPgUUID(serviceID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest appointment visit report by service: %w", err)
	}
	return appointmentVisitReportFromRow(row.AppointmentID, row.OrganizationID, row.Measurements, row.AccessDifficulty, row.Notes, row.CreatedAt, row.UpdatedAt), nil
}

func appointmentVisitReportFromRow(appointmentID, organizationID pgtype.UUID, measurements, accessDifficulty, notes pgtype.Text, createdAt, updatedAt pgtype.Timestamptz) *AppointmentVisitReport {
	return &AppointmentVisitReport{
		AppointmentID:    appointmentID.Bytes,
		OrganizationID:   organizationID.Bytes,
		Measurements:     optionalString(measurements),
		AccessDifficulty: optionalString(accessDifficulty),
		Notes:            optionalString(notes),
		CreatedAt:        createdAt.Time,
		UpdatedAt:        updatedAt.Time,
	}
}
