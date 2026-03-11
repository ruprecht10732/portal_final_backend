package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	identitydb "portal_final_backend/internal/identity/db"
)

type WhatsAppReplyFeedback struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ConversationID  uuid.UUID
	LeadID          uuid.UUID
	LeadServiceID   uuid.UUID
	Scenario        string
	AIReply         string
	HumanReply      string
	WasEdited       bool
	AppliedToMemory bool
	CreatedAt       time.Time
}

type CreateWhatsAppReplyFeedbackParams struct {
	OrganizationID uuid.UUID
	ConversationID uuid.UUID
	LeadID         uuid.UUID
	Scenario       string
	AIReply        string
	HumanReply     string
	WasEdited      bool
}

func (r *Repository) CreateWhatsAppReplyFeedback(ctx context.Context, params CreateWhatsAppReplyFeedbackParams) (*WhatsAppReplyFeedback, error) {
	if params.OrganizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if params.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation_id is required")
	}
	if params.LeadID == uuid.Nil {
		return nil, fmt.Errorf("lead_id is required")
	}
	params.AIReply = strings.TrimSpace(params.AIReply)
	params.HumanReply = strings.TrimSpace(params.HumanReply)
	params.Scenario = strings.TrimSpace(params.Scenario)
	if params.AIReply == "" || params.HumanReply == "" {
		return nil, nil
	}
	if params.Scenario == "" {
		params.Scenario = "generic"
	}

	const query = `
		WITH current_service AS (
			SELECT ls.id AS lead_service_id
			FROM RAC_lead_services ls
			WHERE ls.organization_id = $1
			  AND ls.lead_id = $3
			ORDER BY
			  CASE WHEN ls.pipeline_stage::text NOT IN ('Completed', 'Lost') THEN 0 ELSE 1 END,
			  ls.updated_at DESC,
			  ls.created_at DESC
			LIMIT 1
		)
		INSERT INTO RAC_whatsapp_reply_feedback (
		  organization_id,
		  conversation_id,
		  lead_id,
		  lead_service_id,
		  scenario,
		  ai_reply,
		  human_reply,
		  was_edited
		)
		SELECT $1, $2, $3, current_service.lead_service_id, $4, $5, $6, $7
		FROM current_service
		RETURNING id, organization_id, conversation_id, lead_id, lead_service_id,
		  scenario, ai_reply, human_reply, was_edited, applied_to_memory, created_at`

	var row identitydb.RacWhatsappReplyFeedback
	err := r.pool.QueryRow(ctx, query,
		params.OrganizationID,
		params.ConversationID,
		params.LeadID,
		params.Scenario,
		params.AIReply,
		params.HumanReply,
		params.WasEdited,
	).Scan(
		&row.ID,
		&row.OrganizationID,
		&row.ConversationID,
		&row.LeadID,
		&row.LeadServiceID,
		&row.Scenario,
		&row.AiReply,
		&row.HumanReply,
		&row.WasEdited,
		&row.AppliedToMemory,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	feedback := whatsAppReplyFeedbackFromDB(row)
	return &feedback, nil
}

func (r *Repository) ListRecentAppliedWhatsAppReplyFeedback(ctx context.Context, organizationID uuid.UUID, referenceLeadID *uuid.UUID, excludeConversationID uuid.UUID, limit int) ([]WhatsAppReplyFeedback, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if referenceLeadID == nil || *referenceLeadID == uuid.Nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if limit > 20 {
		limit = 20
	}

	const query = `
		WITH reference_service AS (
		  SELECT st.name AS service_type, ls.pipeline_stage::text AS pipeline_stage
		  FROM RAC_lead_services ls
		  JOIN RAC_service_types st
		    ON st.id = ls.service_type_id
		   AND st.organization_id = ls.organization_id
		  WHERE ls.organization_id = $1
		    AND ls.lead_id = $2
		  ORDER BY
		    CASE WHEN ls.pipeline_stage::text NOT IN ('Completed', 'Lost') THEN 0 ELSE 1 END,
		    ls.updated_at DESC,
		    ls.created_at DESC
		  LIMIT 1
		)
		SELECT f.id, f.organization_id, f.conversation_id, f.lead_id, f.lead_service_id,
		  f.scenario, f.ai_reply, f.human_reply, f.was_edited, f.applied_to_memory, f.created_at
		FROM RAC_whatsapp_reply_feedback f
		JOIN RAC_lead_services ls
		  ON ls.id = f.lead_service_id
		 AND ls.organization_id = f.organization_id
		JOIN RAC_service_types st
		  ON st.id = ls.service_type_id
		 AND st.organization_id = ls.organization_id
		LEFT JOIN reference_service rs ON TRUE
		WHERE f.organization_id = $1
		  AND f.applied_to_memory = true
		  AND f.was_edited = true
		  AND f.conversation_id <> $3
		ORDER BY
		  CASE
		    WHEN rs.service_type IS NOT NULL AND st.name = rs.service_type THEN 0
		    WHEN rs.service_type IS NOT NULL THEN 1
		    ELSE 2
		  END ASC,
		  CASE
		    WHEN rs.pipeline_stage IS NOT NULL AND ls.pipeline_stage::text = rs.pipeline_stage THEN 0
		    WHEN rs.pipeline_stage IS NOT NULL THEN 1
		    ELSE 2
		  END ASC,
		  f.created_at DESC
		LIMIT $4`

	rows, err := r.pool.Query(ctx, query, organizationID, *referenceLeadID, excludeConversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WhatsAppReplyFeedback, 0, limit)
	for rows.Next() {
		var item identitydb.RacWhatsappReplyFeedback
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.ConversationID, &item.LeadID, &item.LeadServiceID, &item.Scenario, &item.AiReply, &item.HumanReply, &item.WasEdited, &item.AppliedToMemory, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, whatsAppReplyFeedbackFromDB(item))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListWhatsAppReplyScenarioAnalytics(ctx context.Context, organizationID uuid.UUID) ([]ReplyScenarioAnalyticsItem, error) {
	const query = `
		SELECT scenario, COUNT(*)::int AS sent_count,
		       COUNT(*) FILTER (WHERE was_edited)::int AS edited_count,
		       MAX(created_at) AS last_used_at
		FROM RAC_whatsapp_reply_feedback
		WHERE organization_id = $1
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
		var lastUsedAt pgtype.Timestamptz
		if err := rows.Scan(&item.Scenario, &item.SentCount, &item.EditedCount, &lastUsedAt); err != nil {
			return nil, err
		}
		if lastUsedAt.Valid {
			value := lastUsedAt.Time
			item.LastUsedAt = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func whatsAppReplyFeedbackFromDB(row identitydb.RacWhatsappReplyFeedback) WhatsAppReplyFeedback {
	return WhatsAppReplyFeedback{
		ID:              uuid.UUID(row.ID.Bytes),
		OrganizationID:  uuid.UUID(row.OrganizationID.Bytes),
		ConversationID:  uuid.UUID(row.ConversationID.Bytes),
		LeadID:          uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:   uuid.UUID(row.LeadServiceID.Bytes),
		Scenario:        row.Scenario,
		AIReply:         row.AiReply,
		HumanReply:      row.HumanReply,
		WasEdited:       row.WasEdited,
		AppliedToMemory: row.AppliedToMemory,
		CreatedAt:       row.CreatedAt.Time,
	}
}
