package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	notificationdb "portal_final_backend/internal/notification/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusEnqueued       Status = "enqueued"
	StatusProcessing     Status = "processing"
	StatusSucceeded      Status = "succeeded"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
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
	LeadID    *uuid.UUID
	ServiceID *uuid.UUID
	Kind      string
	Template  string
	Payload   any
	RunAt     time.Time
	Status    Status // optional; defaults to pending
	LastError *string
}

type Repository struct {
	pool    *pgxpool.Pool
	queries *notificationdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	if pool == nil {
		return &Repository{}
	}
	return &Repository{pool: pool, queries: notificationdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func recordFromModel(model notificationdb.RacNotificationOutbox) Record {
	return Record{
		ID:       uuid.UUID(model.ID.Bytes),
		TenantID: uuid.UUID(model.TenantID.Bytes),
		Kind:     model.Kind,
		Template: model.Template,
		Payload:  json.RawMessage(model.Payload),
		RunAt:    model.RunAt.Time,
		Status:   Status(model.Status),
		Attempts: int(model.Attempts),
	}
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

	id, err := r.queries.InsertNotificationOutbox(ctx, notificationdb.InsertNotificationOutboxParams{
		TenantID:  toPgUUID(p.TenantID),
		LeadID:    toPgUUIDPtr(p.LeadID),
		ServiceID: toPgUUIDPtr(p.ServiceID),
		Kind:      p.Kind,
		Template:  p.Template,
		Payload:   payloadBytes,
		RunAt:     toPgTimestamp(p.RunAt),
		Status:    string(status),
		LastError: toPgText(p.LastError),
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(id.Bytes), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Record, error) {
	if r == nil || r.pool == nil {
		return Record{}, errors.New(errRepoNotConfigured)
	}

	model, err := r.queries.GetNotificationOutboxByID(ctx, toPgUUID(id))
	if err != nil {
		return Record{}, err
	}
	return recordFromModel(model), nil
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

	queries := r.queries.WithTx(tx)
	models, err := queries.ClaimPendingNotificationOutbox(ctx, int32(limit))
	if err != nil {
		return nil, err
	}

	results := make([]Record, 0, len(models))
	for _, model := range models {
		results = append(results, recordFromModel(model))
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
	return r.queries.MarkNotificationOutboxPending(ctx, notificationdb.MarkNotificationOutboxPendingParams{
		LastError: toPgText(lastError),
		ID:        toPgUUID(id),
	})
}

func (r *Repository) ScheduleRetry(ctx context.Context, id uuid.UUID, runAt time.Time, lastError string) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	if runAt.IsZero() {
		runAt = time.Now().UTC()
	}
	return r.queries.ScheduleNotificationOutboxRetry(ctx, notificationdb.ScheduleNotificationOutboxRetryParams{
		RunAt:     toPgTimestamp(runAt),
		LastError: lastError,
		ID:        toPgUUID(id),
	})
}

func (r *Repository) MarkProcessing(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	return r.queries.MarkNotificationOutboxProcessing(ctx, toPgUUID(id))
}

func (r *Repository) MarkSucceeded(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	return r.queries.MarkNotificationOutboxSucceeded(ctx, toPgUUID(id))
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	if r == nil || r.pool == nil {
		return errors.New(errRepoNotConfigured)
	}
	return r.queries.MarkNotificationOutboxFailed(ctx, notificationdb.MarkNotificationOutboxFailedParams{
		LastError: lastError,
		ID:        toPgUUID(id),
	})
}

func (r *Repository) CancelPendingForLead(ctx context.Context, tenantID, leadID uuid.UUID) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New(errRepoNotConfigured)
	}
	if tenantID == uuid.Nil {
		return 0, fmt.Errorf("tenantId is required")
	}
	if leadID == uuid.Nil {
		return 0, fmt.Errorf("leadId is required")
	}

	result, err := r.queries.CancelPendingNotificationOutboxForLead(ctx, notificationdb.CancelPendingNotificationOutboxForLeadParams{
		CancelledStatus: string(StatusCancelled),
		TenantID:        toPgUUID(tenantID),
		LeadID:          toPgUUID(leadID),
		PendingStatus:   string(StatusPending),
	})
	if err != nil {
		return 0, err
	}

	return result, nil
}
