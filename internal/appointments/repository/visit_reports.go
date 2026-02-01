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

type VisitReport struct {
	AppointmentID    uuid.UUID
	Measurements     *string
	AccessDifficulty *string
	Notes            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AppointmentAttachment struct {
	ID            uuid.UUID
	AppointmentID uuid.UUID
	FileKey       string
	FileName      string
	ContentType   *string
	SizeBytes     *int64
	CreatedAt     time.Time
}

func (r *Repository) GetVisitReport(ctx context.Context, appointmentID uuid.UUID) (*VisitReport, error) {
	var report VisitReport
	query := `SELECT appointment_id, measurements, access_difficulty, notes, created_at, updated_at
		FROM appointment_visit_reports WHERE appointment_id = $1`

	err := r.pool.QueryRow(ctx, query, appointmentID).Scan(
		&report.AppointmentID,
		&report.Measurements,
		&report.AccessDifficulty,
		&report.Notes,
		&report.CreatedAt,
		&report.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("visit report not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get visit report: %w", err)
	}

	return &report, nil
}

func (r *Repository) UpsertVisitReport(ctx context.Context, report VisitReport) (*VisitReport, error) {
	query := `
		INSERT INTO appointment_visit_reports
			(appointment_id, measurements, access_difficulty, notes, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, now(), now())
		ON CONFLICT (appointment_id)
		DO UPDATE SET
			measurements = EXCLUDED.measurements,
			access_difficulty = EXCLUDED.access_difficulty,
			notes = EXCLUDED.notes,
			updated_at = now()
		RETURNING appointment_id, measurements, access_difficulty, notes, created_at, updated_at`

	var saved VisitReport
	err := r.pool.QueryRow(ctx, query,
		report.AppointmentID,
		report.Measurements,
		report.AccessDifficulty,
		report.Notes,
	).Scan(
		&saved.AppointmentID,
		&saved.Measurements,
		&saved.AccessDifficulty,
		&saved.Notes,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert visit report: %w", err)
	}

	return &saved, nil
}

func (r *Repository) CreateAttachment(ctx context.Context, attachment AppointmentAttachment) (*AppointmentAttachment, error) {
	query := `
		INSERT INTO appointment_attachments
			(id, appointment_id, file_key, file_name, content_type, size_bytes)
		VALUES
			($1, $2, $3, $4, $5, $6)
		RETURNING id, appointment_id, file_key, file_name, content_type, size_bytes, created_at`

	var saved AppointmentAttachment
	err := r.pool.QueryRow(ctx, query,
		attachment.ID,
		attachment.AppointmentID,
		attachment.FileKey,
		attachment.FileName,
		attachment.ContentType,
		attachment.SizeBytes,
	).Scan(
		&saved.ID,
		&saved.AppointmentID,
		&saved.FileKey,
		&saved.FileName,
		&saved.ContentType,
		&saved.SizeBytes,
		&saved.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create appointment attachment: %w", err)
	}

	return &saved, nil
}

func (r *Repository) ListAttachments(ctx context.Context, appointmentID uuid.UUID) ([]AppointmentAttachment, error) {
	query := `SELECT id, appointment_id, file_key, file_name, content_type, size_bytes, created_at
		FROM appointment_attachments WHERE appointment_id = $1 ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list appointment attachments: %w", err)
	}
	defer rows.Close()

	items := make([]AppointmentAttachment, 0)
	for rows.Next() {
		var item AppointmentAttachment
		if err := rows.Scan(
			&item.ID,
			&item.AppointmentID,
			&item.FileKey,
			&item.FileName,
			&item.ContentType,
			&item.SizeBytes,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan appointment attachment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate appointment attachments: %w", err)
	}

	return items, nil
}
