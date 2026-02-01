package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AvailabilityRule struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Weekday   int
	StartTime time.Time
	EndTime   time.Time
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AvailabilityOverride struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Date        time.Time
	IsAvailable bool
	StartTime   *time.Time
	EndTime     *time.Time
	Timezone    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Repository) CreateAvailabilityRule(ctx context.Context, rule AvailabilityRule) (*AvailabilityRule, error) {
	query := `
		INSERT INTO appointment_availability_rules
			(id, user_id, weekday, start_time, end_time, timezone)
		VALUES
			($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at`

	var saved AvailabilityRule
	err := r.pool.QueryRow(ctx, query,
		rule.ID,
		rule.UserID,
		rule.Weekday,
		rule.StartTime,
		rule.EndTime,
		rule.Timezone,
	).Scan(
		&saved.ID,
		&saved.UserID,
		&saved.Weekday,
		&saved.StartTime,
		&saved.EndTime,
		&saved.Timezone,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create availability rule: %w", err)
	}

	return &saved, nil
}

func (r *Repository) ListAvailabilityRules(ctx context.Context, userID uuid.UUID) ([]AvailabilityRule, error) {
	query := `SELECT id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at
		FROM appointment_availability_rules WHERE user_id = $1 ORDER BY weekday, start_time`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list availability rules: %w", err)
	}
	defer rows.Close()

	items := make([]AvailabilityRule, 0)
	for rows.Next() {
		var item AvailabilityRule
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Weekday,
			&item.StartTime,
			&item.EndTime,
			&item.Timezone,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan availability rule: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate availability rules: %w", err)
	}

	return items, nil
}

func (r *Repository) GetAvailabilityRuleByID(ctx context.Context, id uuid.UUID) (*AvailabilityRule, error) {
	query := `SELECT id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at
		FROM appointment_availability_rules WHERE id = $1`

	var item AvailabilityRule
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.UserID,
		&item.Weekday,
		&item.StartTime,
		&item.EndTime,
		&item.Timezone,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability rule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get availability rule: %w", err)
	}

	return &item, nil
}

func (r *Repository) DeleteAvailabilityRule(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM appointment_availability_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete availability rule: %w", err)
	}
	return nil
}

func (r *Repository) CreateAvailabilityOverride(ctx context.Context, override AvailabilityOverride) (*AvailabilityOverride, error) {
	query := `
		INSERT INTO appointment_availability_overrides
			(id, user_id, date, is_available, start_time, end_time, timezone)
		VALUES
			($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at`

	var saved AvailabilityOverride
	err := r.pool.QueryRow(ctx, query,
		override.ID,
		override.UserID,
		override.Date,
		override.IsAvailable,
		override.StartTime,
		override.EndTime,
		override.Timezone,
	).Scan(
		&saved.ID,
		&saved.UserID,
		&saved.Date,
		&saved.IsAvailable,
		&saved.StartTime,
		&saved.EndTime,
		&saved.Timezone,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create availability override: %w", err)
	}

	return &saved, nil
}

func (r *Repository) ListAvailabilityOverrides(ctx context.Context, userID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]AvailabilityOverride, error) {
	baseQuery := `SELECT id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at
		FROM appointment_availability_overrides WHERE user_id = $1`
	args := []interface{}{userID}
	argIndex := 2

	if startDate != nil {
		baseQuery += fmt.Sprintf(" AND date >= $%d", argIndex)
		args = append(args, *startDate)
		argIndex++
	}
	if endDate != nil {
		baseQuery += fmt.Sprintf(" AND date <= $%d", argIndex)
		args = append(args, *endDate)
		argIndex++
	}

	baseQuery += " ORDER BY date ASC"

	rows, err := r.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list availability overrides: %w", err)
	}
	defer rows.Close()

	items := make([]AvailabilityOverride, 0)
	for rows.Next() {
		var item AvailabilityOverride
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Date,
			&item.IsAvailable,
			&item.StartTime,
			&item.EndTime,
			&item.Timezone,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan availability override: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate availability overrides: %w", err)
	}

	return items, nil
}

func (r *Repository) GetAvailabilityOverrideByID(ctx context.Context, id uuid.UUID) (*AvailabilityOverride, error) {
	query := `SELECT id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at
		FROM appointment_availability_overrides WHERE id = $1`

	var item AvailabilityOverride
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.UserID,
		&item.Date,
		&item.IsAvailable,
		&item.StartTime,
		&item.EndTime,
		&item.Timezone,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability override not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get availability override: %w", err)
	}

	return &item, nil
}

func (r *Repository) DeleteAvailabilityOverride(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM appointment_availability_overrides WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete availability override: %w", err)
	}
	return nil
}
