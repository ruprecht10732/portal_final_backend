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
	ConsumerNote          *string
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
	LeadID       uuid.UUID
	ServiceType  string
	ConsumerNote *string
}

func (r *Repository) CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO lead_services (lead_id, service_type_id, status, consumer_note)
			VALUES (
				$1,
				(SELECT id FROM service_types WHERE name = $2 OR slug = $2 LIMIT 1),
				'New',
				$3
			)
			RETURNING *
		)
		SELECT i.id, i.lead_id, st.name AS service_type, i.status, i.consumer_note,
			i.visit_scheduled_date, i.visit_scout_id, i.visit_measurements, i.visit_access_difficulty, i.visit_notes, i.visit_completed_at,
			i.created_at, i.updated_at
		FROM inserted i
		JOIN service_types st ON st.id = i.service_type_id
	`, params.LeadID, params.ServiceType, params.ConsumerNote).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	return svc, err
}

func (r *Repository) GetLeadServiceByID(ctx context.Context, id uuid.UUID) (LeadService, error) {
	var svc LeadService
	err := r.pool.QueryRow(ctx, `
		SELECT ls.id, ls.lead_id, st.name AS service_type, ls.status, ls.consumer_note,
			ls.visit_scheduled_date, ls.visit_scout_id, ls.visit_measurements, ls.visit_access_difficulty, ls.visit_notes, ls.visit_completed_at,
			ls.created_at, ls.updated_at
		FROM lead_services ls
		JOIN service_types st ON st.id = ls.service_type_id
		WHERE ls.id = $1
	`, id).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		SELECT ls.id, ls.lead_id, st.name AS service_type, ls.status, ls.consumer_note,
			ls.visit_scheduled_date, ls.visit_scout_id, ls.visit_measurements, ls.visit_access_difficulty, ls.visit_notes, ls.visit_completed_at,
			ls.created_at, ls.updated_at
		FROM lead_services ls
		JOIN service_types st ON st.id = ls.service_type_id
		WHERE ls.lead_id = $1
		ORDER BY ls.created_at DESC
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	services := make([]LeadService, 0)
	for rows.Next() {
		var svc LeadService
		if err := rows.Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		SELECT ls.id, ls.lead_id, st.name AS service_type, ls.status, ls.consumer_note,
			ls.visit_scheduled_date, ls.visit_scout_id, ls.visit_measurements, ls.visit_access_difficulty, ls.visit_notes, ls.visit_completed_at,
			ls.created_at, ls.updated_at
		FROM lead_services ls
		JOIN service_types st ON st.id = ls.service_type_id
		WHERE ls.lead_id = $1 AND ls.status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
		ORDER BY ls.created_at DESC
		LIMIT 1
	`, leadID).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
		&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
		&svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Fallback to most recent service of any status
		err = r.pool.QueryRow(ctx, `
			SELECT ls.id, ls.lead_id, st.name AS service_type, ls.status, ls.consumer_note,
				ls.visit_scheduled_date, ls.visit_scout_id, ls.visit_measurements, ls.visit_access_difficulty, ls.visit_notes, ls.visit_completed_at,
				ls.created_at, ls.updated_at
			FROM lead_services ls
			JOIN service_types st ON st.id = ls.service_type_id
			WHERE ls.lead_id = $1
			ORDER BY ls.created_at DESC
			LIMIT 1
		`, leadID).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		WITH updated AS (
			UPDATE lead_services SET %s
			WHERE id = $%d
			RETURNING *
		)
		SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
			u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
			u.created_at, u.updated_at
		FROM updated u
		JOIN service_types st ON st.id = u.service_type_id
	`, strings.Join(setClauses, ", "), argIdx)

	var svc LeadService
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		WITH updated AS (
			UPDATE lead_services SET status = $2, updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
			u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
			u.created_at, u.updated_at
		FROM updated u
		JOIN service_types st ON st.id = u.service_type_id
	`, id, status).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		WITH updated AS (
			UPDATE lead_services SET 
				visit_scheduled_date = $2, 
				visit_scout_id = $3,
				status = 'Scheduled',
				updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
			u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
			u.created_at, u.updated_at
		FROM updated u
		JOIN service_types st ON st.id = u.service_type_id
	`, id, scheduledDate, scoutID).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		WITH updated AS (
			UPDATE lead_services SET 
				visit_measurements = $2,
				visit_access_difficulty = $3,
				visit_notes = $4,
				visit_completed_at = now(),
				status = 'Surveyed',
				updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
			u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
			u.created_at, u.updated_at
		FROM updated u
		JOIN service_types st ON st.id = u.service_type_id
	`, id, measurements, accessDifficulty, notesPtr).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
		WITH updated AS (
			UPDATE lead_services SET 
				visit_notes = COALESCE(visit_notes || E'\n', '') || COALESCE($2, 'No show'),
				status = 'Needs_Rescheduling',
				updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
			u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
			u.created_at, u.updated_at
		FROM updated u
		JOIN service_types st ON st.id = u.service_type_id
	`, id, notesPtr).Scan(
		&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
			WITH updated AS (
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
				RETURNING *
			)
			SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
				u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
				u.created_at, u.updated_at
			FROM updated u
			JOIN service_types st ON st.id = u.service_type_id
		`, id, scheduledDate, scoutID, noShowNote).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
			&svc.VisitScheduledDate, &svc.VisitScoutID, &svc.VisitMeasurements, &svc.VisitAccessDifficulty, &svc.VisitNotes, &svc.VisitCompletedAt,
			&svc.CreatedAt, &svc.UpdatedAt,
		)
	} else {
		// Simple reschedule without no-show
		err = r.pool.QueryRow(ctx, `
			WITH updated AS (
				UPDATE lead_services SET 
					visit_scheduled_date = $2,
					visit_scout_id = $3,
					visit_measurements = NULL,
					visit_access_difficulty = NULL,
					visit_completed_at = NULL,
					status = 'Scheduled',
					updated_at = now()
				WHERE id = $1
				RETURNING *
			)
			SELECT u.id, u.lead_id, st.name AS service_type, u.status, u.consumer_note,
				u.visit_scheduled_date, u.visit_scout_id, u.visit_measurements, u.visit_access_difficulty, u.visit_notes, u.visit_completed_at,
				u.created_at, u.updated_at
			FROM updated u
			JOIN service_types st ON st.id = u.service_type_id
		`, id, scheduledDate, scoutID).Scan(
			&svc.ID, &svc.LeadID, &svc.ServiceType, &svc.Status, &svc.ConsumerNote,
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
