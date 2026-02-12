package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusEnqueued       Status = "enqueued"
	StatusProcessing     Status = "processing"
	StatusSucceeded      Status = "succeeded"
	StatusFailed         Status = "failed"
	errRepoNotConfigured        = "outbox repository not configured"
)

type Record struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Kind     string
	Template string
	Payload  json.RawMessage
	RunAt    time.Time
	Status   Status
	Attempts int
}

type InsertParams struct {
	TenantID  uuid.UUID
	Kind      string
	Template  string
	Payload   any
	RunAt     time.Time
	Status    Status // optional; defaults to pending
	LastError *string
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Insert(ctx context.Context, p InsertParams) (uuid.UUID, error) {
	if r == nil || r.pool == nil {
		return uuid.Nil, errors.New(errRepoNotConfigured)
	}
	if p.TenantID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("tenantId is required")
	}
	if p.Kind == "" {
		return uuid.Nil, fmt.Errorf("kind is required")
	}
	if p.Template == "" {
		return uuid.Nil, fmt.Errorf("template is required")
	}
	if p.RunAt.IsZero() {
		p.RunAt = time.Now().UTC()
	}
	status := p.Status
	if status == "" {
		status = StatusPending
	}

	payloadBytes, err := json.Marshal(p.Payload)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal payload: %w", err)
	}

	var id uuid.UUID
	err = r.pool.QueryRow(ctx,
		`INSERT INTO RAC_notification_outbox (tenant_id, kind, template, payload, run_at, status, last_error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		p.TenantID, p.Kind, p.Template, payloadBytes, p.RunAt, string(status), p.LastError,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Record, error) {
	if r == nil || r.pool == nil {
		return Record{}, errors.New(errRepoNotConfigured)
	}

	var rec Record
	var status string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, kind, template, payload, run_at, status, attempts
		 FROM RAC_notification_outbox
		 WHERE id = $1`,
		id,
	).Scan(&rec.ID, &rec.TenantID, &rec.Kind, &rec.Template, &rec.Payload, &rec.RunAt, &status, &rec.Attempts)
	if err != nil {
		return Record{}, err
	}
	rec.Status = Status(status)
	return rec, nil
}

func (r *Repository) ClaimPending(ctx context.Context, limit int) ([]Record, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New(errRepoNotConfigured)
	}
	if limit < 1 {
		limit = 50
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `WITH cte AS (
		SELECT id
		FROM RAC_notification_outbox
		WHERE status = 'pending'
		ORDER BY run_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	)
	UPDATE RAC_notification_outbox o
	SET status = 'enqueued', updated_at = now()
	FROM cte
	WHERE o.id = cte.id
	RETURNING o.id, o.tenant_id, o.kind, o.template, o.payload, o.run_at, o.status, o.attempts`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Record
	for rows.Next() {
		var rec Record
		var status string
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.Kind, &rec.Template, &rec.Payload, &rec.RunAt, &status, &rec.Attempts); err != nil {
			return nil, err
		}
		rec.Status = Status(status)
		results = append(results, rec)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) MarkPending(ctx context.Context, id uuid.UUID, lastError *string) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE RAC_notification_outbox
		 SET status = 'pending', last_error = $2, updated_at = now()
		 WHERE id = $1`,
		id, lastError,
	)
	return err
}

func (r *Repository) MarkProcessing(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE RAC_notification_outbox
		 SET status = 'processing', attempts = attempts + 1, updated_at = now()
		 WHERE id = $1`,
		id,
	)
	return err
}

func (r *Repository) MarkSucceeded(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE RAC_notification_outbox
		 SET status = 'succeeded', last_error = NULL, updated_at = now()
		 WHERE id = $1`,
		id,
	)
	return err
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE RAC_notification_outbox
		 SET status = 'failed', last_error = $2, updated_at = now()
		 WHERE id = $1`,
		id, lastError,
	)
	return err
}
