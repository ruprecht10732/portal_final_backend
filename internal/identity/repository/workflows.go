package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type NotificationWorkflow struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Trigger        string
	Channel        string
	Audience       string
	Enabled        bool
	DelayMinutes   int
	LeadSource     *string
	TemplateText   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type NotificationWorkflowUpsert struct {
	Trigger      string
	Channel      string
	Audience     string
	Enabled      bool
	DelayMinutes int
	LeadSource   *string
	TemplateText *string
}

func (r *Repository) ListNotificationWorkflows(ctx context.Context, organizationID uuid.UUID) ([]NotificationWorkflow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, trigger, channel, audience, enabled, delay_minutes, lead_source, template_text, created_at, updated_at
		FROM RAC_notification_workflows
		WHERE organization_id = $1
		ORDER BY trigger ASC, channel ASC, audience ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NotificationWorkflow
	for rows.Next() {
		var w NotificationWorkflow
		if err := rows.Scan(
			&w.ID,
			&w.OrganizationID,
			&w.Trigger,
			&w.Channel,
			&w.Audience,
			&w.Enabled,
			&w.DelayMinutes,
			&w.LeadSource,
			&w.TemplateText,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (r *Repository) ReplaceNotificationWorkflows(ctx context.Context, organizationID uuid.UUID, workflows []NotificationWorkflowUpsert) ([]NotificationWorkflow, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM RAC_notification_workflows WHERE organization_id = $1`, organizationID); err != nil {
		return nil, err
	}

	for _, w := range workflows {
		if w.Channel == "" {
			w.Channel = "whatsapp"
		}
		if w.Audience == "" {
			w.Audience = "lead"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO RAC_notification_workflows
				(organization_id, trigger, channel, audience, enabled, delay_minutes, lead_source, template_text)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8)
		`, organizationID, w.Trigger, w.Channel, w.Audience, w.Enabled, w.DelayMinutes, w.LeadSource, w.TemplateText); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Re-read after commit so callers get IDs/timestamps.
	result, err := r.ListNotificationWorkflows(ctx, organizationID)
	if err != nil {
		// If table doesn't exist yet (migrations not applied), surface a reasonable error.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}
