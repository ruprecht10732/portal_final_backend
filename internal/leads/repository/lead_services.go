package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrServiceNotFound = errors.New("lead service not found")

type LeadService struct {
	ID                    uuid.UUID
	LeadID                uuid.UUID
	ServiceType           string
	Status                string
	VisitScheduledDate    *time.Time
	VisitScoutID          *uuid.UUID
	VisitMeasurements     *string
	VisitAccessDifficulty *string
	VisitNotes            *string
	VisitCompletedAt      *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateLeadServiceParams struct {
	LeadID      uuid.UUID
	ServiceType string
}

func (r *Repository) CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lead_services (lead_id, service_type, status)
		VALUES ($1, $2, 'New')
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, params.LeadID, params.ServiceType).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	return svc, err
}

func (r *Repository) GetLeadServiceByID(ctx context.Context, id uuid.UUID) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM lead_services WHERE id = $1
	`, id).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) ListLeadServices(ctx context.Context, leadID uuid.UUID) ([]LeadService, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM lead_services WHERE lead_id = $1
		ORDER BY created_at DESC
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	services := make([]LeadService, 0)
	for rows.Next() {
		var svc LeadService
		if err := rows.Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
			&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
			&svc.CreatedAt, &svc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

// GetCurrentLeadService returns the most recent non-terminal (not Closed, not Bad_Lead, not Surveyed) service,
// or falls back to the most recent service if all are terminal.
func (r *Repository) GetCurrentLeadService(ctx context.Context, leadID uuid.UUID) (LeadService, error) {
	var svc LeadService
	// Try to find an active (non-terminal) service first
	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM lead_services 
		WHERE lead_id = $1 AND status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
		ORDER BY created_at DESC
		LIMIT 1
	`, leadID).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Fallback to most recent service of any status
		err = r.pool.QueryRow(ctx, `
			SELECT id, lead_id, service_type, status,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
			FROM lead_services 
			WHERE lead_id = $1
			ORDER BY created_at DESC
			LIMIT 1
		`, leadID).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
			&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
			&svc.CreatedAt, &svc.UpdatedAt,
		)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

type UpdateLeadServiceParams struct {
	Status *string
}

func (r *Repository) UpdateLeadService(ctx context.Context, id uuid.UUID, params UpdateLeadServiceParams) (LeadService, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if params.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *params.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetLeadServiceByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE lead_services SET %s
		WHERE id = $%d
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx)

	var svc LeadService
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) UpdateServiceStatus(ctx context.Context, id uuid.UUID, status string) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		UPDATE lead_services SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, status).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) ScheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		UPDATE lead_services SET 
			visit_scheduled_date = $2, 
			visit_scout_id = $3,
			status = 'Scheduled',
			updated_at = now()
		WHERE id = $1
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, scheduledDate, scoutID).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) CompleteServiceSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (LeadService, error) {
	var svc LeadService
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE lead_services SET 
			visit_measurements = $2,
			visit_access_difficulty = $3,
			visit_notes = $4,
			visit_completed_at = now(),
			status = 'Surveyed',
			updated_at = now()
		WHERE id = $1
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, measurements, accessDifficulty, notesPtr).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) MarkServiceNoShow(ctx context.Context, id uuid.UUID, notes string) (LeadService, error) {
	var svc LeadService
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE lead_services SET 
			visit_notes = COALESCE(visit_notes || E'\n', '') || COALESCE($2, 'No show'),
			status = 'Needs_Rescheduling',
			updated_at = now()
		WHERE id = $1
		RETURNING id, lead_id, service_type, status,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, notesPtr).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

func (r *Repository) RescheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (LeadService, error) {
	var svc LeadService
	var err error

	if markAsNoShow {
		// Build no-show note
		noShowNote := "No show"
		if noShowNotes != "" {
			noShowNote = "No show: " + noShowNotes
		}

		err = r.pool.QueryRow(ctx, `
			UPDATE lead_services SET 
				visit_notes = COALESCE(visit_notes || E'\n', '') || $4,
				visit_scheduled_date = $2,
				visit_scout_id = $3,
				visit_measurements = NULL,
				visit_access_difficulty = NULL,
				visit_completed_at = NULL,
				status = 'Scheduled',
				updated_at = now()
			WHERE id = $1
			RETURNING id, lead_id, service_type, status,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID, noShowNote).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
			&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
			&svc.CreatedAt, &svc.UpdatedAt,
		)
	} else {
		// Simple reschedule without no-show
		err = r.pool.QueryRow(ctx, `
			UPDATE lead_services SET 
				visit_scheduled_date = $2,
				visit_scout_id = $3,
				visit_measurements = NULL,
				visit_access_difficulty = NULL,
				visit_completed_at = NULL,
				status = 'Scheduled',
				updated_at = now()
			WHERE id = $1
			RETURNING id, lead_id, service_type, status,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status,
			&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
			&svc.CreatedAt, &svc.UpdatedAt,
		)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return LeadService{}, ErrServiceNotFound
	}
	return svc, err
}

// CloseAllActiveServices marks all non-terminal services for a lead as Closed
func (r *Repository) CloseAllActiveServices(ctx context.Context, leadID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE lead_services 
		SET status = 'Closed', updated_at = now()
		WHERE lead_id = $1 AND status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
	`, leadID)
	return err
}
