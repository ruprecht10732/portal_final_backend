package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type VisitHistory struct {
	ID               uuid.UUID
	LeadID           uuid.UUID
	ScheduledDate    time.Time
	ScoutID          *uuid.UUID
	Outcome          string
	Measurements     *string
	AccessDifficulty *string
	Notes            *string
	CompletedAt      *time.Time
	CreatedAt        time.Time
}

type CreateVisitHistoryParams struct {
	LeadID           uuid.UUID
	ScheduledDate    time.Time
	ScoutID          *uuid.UUID
	Outcome          string
	Measurements     *string
	AccessDifficulty *string
	Notes            *string
	CompletedAt      *time.Time
}

func (r *Repository) CreateVisitHistory(ctx context.Context, params CreateVisitHistoryParams) (VisitHistory, error) {
	var vh VisitHistory
	err := r.pool.QueryRow(ctx, `
		INSERT INTO visit_history (lead_id, scheduled_date, scout_id, outcome, measurements, access_difficulty, notes, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, lead_id, scheduled_date, scout_id, outcome, measurements, access_difficulty, notes, completed_at, created_at
	`,
		params.LeadID, params.ScheduledDate, params.ScoutID, params.Outcome,
		params.Measurements, params.AccessDifficulty, params.Notes, params.CompletedAt,
	).Scan(
		&vh.ID, &vh.LeadID, &vh.ScheduledDate, &vh.ScoutID, &vh.Outcome,
		&vh.Measurements, &vh.AccessDifficulty, &vh.Notes, &vh.CompletedAt, &vh.CreatedAt,
	)
	return vh, err
}

func (r *Repository) ListVisitHistory(ctx context.Context, leadID uuid.UUID) ([]VisitHistory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, scheduled_date, scout_id, outcome, measurements, access_difficulty, notes, completed_at, created_at
		FROM visit_history
		WHERE lead_id = $1
		ORDER BY scheduled_date DESC
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []VisitHistory
	for rows.Next() {
		var vh VisitHistory
		if err := rows.Scan(
			&vh.ID, &vh.LeadID, &vh.ScheduledDate, &vh.ScoutID, &vh.Outcome,
			&vh.Measurements, &vh.AccessDifficulty, &vh.Notes, &vh.CompletedAt, &vh.CreatedAt,
		); err != nil {
			return nil, err
		}
		history = append(history, vh)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return history, nil
}

func (r *Repository) GetVisitHistoryByID(ctx context.Context, id uuid.UUID) (VisitHistory, error) {
	var vh VisitHistory
	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, scheduled_date, scout_id, outcome, measurements, access_difficulty, notes, completed_at, created_at
		FROM visit_history
		WHERE id = $1
	`, id).Scan(
		&vh.ID, &vh.LeadID, &vh.ScheduledDate, &vh.ScoutID, &vh.Outcome,
		&vh.Measurements, &vh.AccessDifficulty, &vh.Notes, &vh.CompletedAt, &vh.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return VisitHistory{}, ErrNotFound
	}
	return vh, err
}
