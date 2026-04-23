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

// ──────────────────────────────────────────────────
// Appointment Stats
// ──────────────────────────────────────────────────

type LeadAppointmentStats struct {
	Total       int
	Scheduled   int
	Completed   int
	Cancelled   int
	HasUpcoming bool
}

// GetLeadAppointmentStats returns appointment statistics for a lead (for scoring purposes).
func (r *Repository) GetLeadAppointmentStats(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadAppointmentStats, error) {
	row, err := r.queries.GetLeadAppointmentStats(ctx, leadsdb.GetLeadAppointmentStatsParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadAppointmentStats{}, nil
	}
	if err != nil {
		return LeadAppointmentStats{}, fmt.Errorf("get lead appointment stats: %w", err)
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
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}

	return optionalTime(row.StartTime), row.Status, nil
}

// ──────────────────────────────────────────────────
// Visit Reports
// ──────────────────────────────────────────────────

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
		return nil, fmt.Errorf("get appointment visit report: %w", err)
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
		return nil, fmt.Errorf("get latest appointment visit report by service: %w", err)
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

// ──────────────────────────────────────────────────
// Service State Aggregates
// ──────────────────────────────────────────────────

// ServiceStateAggregates holds counts and timestamps to determine service state contextually.
type ServiceStateAggregates struct {
	AcceptedQuotes        int
	SentQuotes            int
	DraftQuotes           int
	RejectedQuotes        int
	LatestQuoteAt         *time.Time
	AcceptedOffers        int
	PendingOffers         int
	LatestOfferAt         *time.Time
	ScheduledAppointments int
	CompletedAppointments int
	CancelledAppointments int
	LatestAppointmentAt   *time.Time
	HasVisitReport        bool
	AiAction              *string
	TerminalAt            *time.Time
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
