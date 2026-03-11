package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const errOrganizationIDRequired = "organization_id is required"

type EmailReplyFeedback struct {
	Scenario   string
	AIReply    string
	HumanReply string
	WasEdited  bool
	CreatedAt  time.Time
}

type EmailReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type ReplyScenarioAnalyticsItem struct {
	Scenario    string
	SentCount   int
	EditedCount int
	LastUsedAt  *time.Time
}

type EmailReplyReference struct {
	LeadID        *uuid.UUID
	LeadServiceID *uuid.UUID
}

type CreateEmailReplyFeedbackParams struct {
	OrganizationID  uuid.UUID
	AccountID       uuid.UUID
	SourceUID       int64
	CustomerEmail   string
	CustomerName    *string
	Subject         *string
	CustomerMessage string
	Scenario        string
	AIReply         *string
	HumanReply      string
	WasEdited       bool
	ReplyAll        bool
}

func (r *Repository) CreateEmailReplyFeedback(ctx context.Context, params CreateEmailReplyFeedbackParams) (*EmailReplyFeedback, error) {
	if params.OrganizationID == uuid.Nil {
		return nil, fmt.Errorf(errOrganizationIDRequired)
	}
	if params.AccountID == uuid.Nil {
		return nil, fmt.Errorf("account_id is required")
	}
	if params.SourceUID <= 0 {
		return nil, fmt.Errorf("source_uid is required")
	}
	params.CustomerEmail = strings.ToLower(strings.TrimSpace(params.CustomerEmail))
	params.CustomerMessage = strings.TrimSpace(params.CustomerMessage)
	params.HumanReply = strings.TrimSpace(params.HumanReply)
	params.Scenario = strings.TrimSpace(params.Scenario)
	if params.CustomerEmail == "" || params.CustomerMessage == "" || params.HumanReply == "" {
		return nil, nil
	}
	if params.Scenario == "" {
		params.Scenario = "generic"
	}
	if params.AIReply != nil {
		trimmed := strings.TrimSpace(*params.AIReply)
		if trimmed == "" {
			params.AIReply = nil
		} else {
			params.AIReply = &trimmed
		}
	}

	const query = `
		WITH matched_lead AS (
			SELECT l.id
			FROM RAC_leads l
			WHERE l.organization_id = $1
			  AND LOWER(BTRIM(COALESCE(l.consumer_email, ''))) = LOWER(BTRIM($4))
			ORDER BY l.created_at DESC
			LIMIT 1
		),
		matched_service AS (
			SELECT ls.id
			FROM RAC_lead_services ls
			JOIN matched_lead ml ON ml.id = ls.lead_id
			WHERE ls.organization_id = $1
			ORDER BY
				CASE WHEN ls.pipeline_stage::text NOT IN ('Completed', 'Lost') THEN 0 ELSE 1 END,
				ls.updated_at DESC,
				ls.created_at DESC
			LIMIT 1
		)
		INSERT INTO RAC_email_reply_feedback (
			organization_id,
			account_id,
			source_message_uid,
			customer_email,
			customer_name,
			subject,
			customer_message,
			scenario,
			ai_reply,
			human_reply,
			was_edited,
			reply_all,
			lead_id,
			lead_service_id
		) VALUES (
			$1,
			$2,
			$3,
			LOWER(BTRIM($4)),
			NULLIF(BTRIM($5), ''),
			NULLIF(BTRIM($6), ''),
			$7,
			NULLIF(BTRIM($8), ''),
			$9,
			$10,
			$11,
			(SELECT id FROM matched_lead),
			(SELECT id FROM matched_service)
		)
		RETURNING scenario, ai_reply, human_reply, was_edited, created_at`

	var aiReply pgtype.Text
	var scenario string
	var humanReply string
	var wasEdited bool
	var createdAt time.Time
	err := r.pool.QueryRow(
		ctx,
		query,
		params.OrganizationID,
		params.AccountID,
		params.SourceUID,
		params.CustomerEmail,
		nullableStringValue(params.CustomerName),
		nullableStringValue(params.Subject),
		params.CustomerMessage,
		params.Scenario,
		nullableStringValue(params.AIReply),
		params.HumanReply,
		params.WasEdited,
		params.ReplyAll,
	).Scan(&scenario, &aiReply, &humanReply, &wasEdited, &createdAt)
	if err != nil {
		return nil, err
	}

	item := &EmailReplyFeedback{Scenario: scenario, HumanReply: humanReply, WasEdited: wasEdited, CreatedAt: createdAt}
	if aiReply.Valid {
		item.AIReply = aiReply.String
	}
	return item, nil
}

func (r *Repository) ListRecentAppliedEmailReplyFeedback(ctx context.Context, organizationID uuid.UUID, reference EmailReplyReference, customerEmail string, excludeAccountID uuid.UUID, excludeUID int64, limit int) ([]EmailReplyFeedback, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf(errOrganizationIDRequired)
	}
	customerEmail = strings.ToLower(strings.TrimSpace(customerEmail))
	if customerEmail == "" && reference.LeadID == nil && reference.LeadServiceID == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if limit > 20 {
		limit = 20
	}

	const query = `
		SELECT scenario, ai_reply, human_reply, was_edited, created_at
		FROM RAC_email_reply_feedback
		WHERE organization_id = $1
		  AND (
				($2::uuid IS NOT NULL AND lead_service_id = $2::uuid)
				OR ($3::uuid IS NOT NULL AND lead_id = $3::uuid)
				OR ($4 <> '' AND customer_email = LOWER(BTRIM($4)))
		  )
		  AND applied_to_memory = TRUE
		  AND ai_reply IS NOT NULL
		  AND BTRIM(ai_reply) <> ''
		  AND BTRIM(ai_reply) <> BTRIM(human_reply)
		  AND NOT (account_id = $5 AND source_message_uid = $6)
		ORDER BY
		  CASE
				WHEN $2::uuid IS NOT NULL AND lead_service_id = $2::uuid THEN 0
				WHEN $3::uuid IS NOT NULL AND lead_id = $3::uuid THEN 1
				WHEN $4 <> '' AND customer_email = LOWER(BTRIM($4)) THEN 2
				ELSE 3
		  END,
		  created_at DESC
		LIMIT $7`

	rows, err := r.pool.Query(ctx, query, organizationID, nullableUUIDValue(reference.LeadServiceID), nullableUUIDValue(reference.LeadID), customerEmail, excludeAccountID, excludeUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]EmailReplyFeedback, 0, limit)
	for rows.Next() {
		var item EmailReplyFeedback
		if err := rows.Scan(&item.Scenario, &item.AIReply, &item.HumanReply, &item.WasEdited, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListEmailReplyScenarioAnalytics(ctx context.Context, organizationID uuid.UUID) ([]ReplyScenarioAnalyticsItem, error) {
	const query = `
		SELECT scenario, COUNT(*)::int AS sent_count,
		       COUNT(*) FILTER (WHERE was_edited)::int AS edited_count,
		       MAX(created_at) AS last_used_at
		FROM RAC_email_reply_feedback
		WHERE organization_id = $1
		  AND ai_reply IS NOT NULL
		  AND BTRIM(ai_reply) <> ''
		GROUP BY scenario
		ORDER BY sent_count DESC, scenario ASC`

	rows, err := r.pool.Query(ctx, query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ReplyScenarioAnalyticsItem, 0)
	for rows.Next() {
		var item ReplyScenarioAnalyticsItem
		var lastUsedAt time.Time
		if err := rows.Scan(&item.Scenario, &item.SentCount, &item.EditedCount, &lastUsedAt); err != nil {
			return nil, err
		}
		item.LastUsedAt = &lastUsedAt
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListRecentEmailReplyExamples(ctx context.Context, organizationID uuid.UUID, reference EmailReplyReference, customerEmail string, excludeAccountID uuid.UUID, excludeUID int64, limit int) ([]EmailReplyExample, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf(errOrganizationIDRequired)
	}
	customerEmail = strings.ToLower(strings.TrimSpace(customerEmail))
	if customerEmail == "" && reference.LeadID == nil && reference.LeadServiceID == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if limit > 20 {
		limit = 20
	}

	const query = `
		SELECT customer_message, human_reply, created_at
		FROM RAC_email_reply_feedback
		WHERE organization_id = $1
		  AND (
				($2::uuid IS NOT NULL AND lead_service_id = $2::uuid)
				OR ($3::uuid IS NOT NULL AND lead_id = $3::uuid)
				OR ($4 <> '' AND customer_email = LOWER(BTRIM($4)))
		  )
		  AND applied_to_memory = TRUE
		  AND BTRIM(customer_message) <> ''
		  AND BTRIM(human_reply) <> ''
		  AND NOT (account_id = $5 AND source_message_uid = $6)
		ORDER BY
		  CASE
				WHEN $2::uuid IS NOT NULL AND lead_service_id = $2::uuid THEN 0
				WHEN $3::uuid IS NOT NULL AND lead_id = $3::uuid THEN 1
				WHEN $4 <> '' AND customer_email = LOWER(BTRIM($4)) THEN 2
				ELSE 3
		  END,
		  created_at DESC
		LIMIT $7`

	rows, err := r.pool.Query(ctx, query, organizationID, nullableUUIDValue(reference.LeadServiceID), nullableUUIDValue(reference.LeadID), customerEmail, excludeAccountID, excludeUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]EmailReplyExample, 0, limit)
	for rows.Next() {
		var item EmailReplyExample
		if err := rows.Scan(&item.CustomerMessage, &item.Reply, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableUUIDValue(value *uuid.UUID) any {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return *value
}
