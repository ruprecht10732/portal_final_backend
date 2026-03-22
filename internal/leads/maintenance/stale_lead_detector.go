package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/logger"
)

// StaleReason describes why a lead service is considered stale.
type StaleReason string

const (
	StaleReasonNoActivity        StaleReason = "no_activity"
	StaleReasonStuckNurturing    StaleReason = "stuck_nurturing"
	StaleReasonNoQuoteSent       StaleReason = "no_quote_sent"
	StaleReasonStaleDraft        StaleReason = "stale_draft"
	StaleReasonNeedsRescheduling StaleReason = "needs_rescheduling"
)

// StaleLeadItem represents a single stale lead service with context.
type StaleLeadItem struct {
	LeadID            uuid.UUID
	ServiceID         uuid.UUID
	OrganizationID    uuid.UUID
	StaleReason       StaleReason
	PipelineStage     string
	Status            string
	LastActivityAt    *time.Time
	ConsumerFirstName string
	ConsumerLastName  string
	ConsumerPhone     string
	ConsumerEmail     *string
	ServiceType       string
}

// StaleLeadDetector queries the database for stale lead services.
type StaleLeadDetector struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewStaleLeadDetector creates a new StaleLeadDetector.
func NewStaleLeadDetector(pool *pgxpool.Pool, log *logger.Logger) *StaleLeadDetector {
	return &StaleLeadDetector{pool: pool, log: log}
}

const staleLeadQuery = `
WITH last_activity AS (
	SELECT
		te.service_id,
		MAX(te.created_at) AS last_activity_at
	FROM lead_timeline_events te
	WHERE te.organization_id = $1
		AND te.service_id IS NOT NULL
	GROUP BY te.service_id
),
candidates AS (
	SELECT
		l.id AS lead_id,
		s.id AS service_id,
		s.organization_id,
		CASE
			WHEN s.status = 'Needs_Rescheduling'
				AND s.updated_at < NOW() - INTERVAL '2 days'
				THEN 'needs_rescheduling'
			WHEN s.pipeline_stage = 'Nurturing'
				AND s.status = 'Attempted_Contact'
				AND s.updated_at < NOW() - INTERVAL '7 days'
				THEN 'stuck_nurturing'
			WHEN s.pipeline_stage IN ('Estimation', 'Proposal')
				AND NOT EXISTS (
					SELECT 1 FROM RAC_quotes q
					WHERE q.lead_service_id = s.id
						AND q.organization_id = s.organization_id
						AND q.status = 'Sent'
				)
				AND s.updated_at < NOW() - INTERVAL '14 days'
				THEN 'no_quote_sent'
			WHEN EXISTS (
					SELECT 1 FROM RAC_quotes q
					WHERE q.lead_service_id = s.id
						AND q.organization_id = s.organization_id
						AND q.status = 'Draft'
						AND q.created_at < NOW() - INTERVAL '30 days'
				)
				THEN 'stale_draft'
			WHEN la.last_activity_at < NOW() - INTERVAL '7 days'
				THEN 'no_activity'
			WHEN la.last_activity_at IS NULL
				AND s.created_at < NOW() - INTERVAL '7 days'
				THEN 'no_activity'
			ELSE NULL
		END AS stale_reason,
		s.pipeline_stage,
		s.status,
		la.last_activity_at,
		l.consumer_first_name,
		l.consumer_last_name,
		l.consumer_phone,
		l.consumer_email,
		st.name AS service_type
	FROM RAC_lead_services s
	JOIN RAC_leads l ON l.id = s.lead_id
	JOIN RAC_service_types st ON st.id = s.service_type_id AND st.organization_id = s.organization_id
	LEFT JOIN last_activity la ON la.service_id = s.id
	WHERE s.organization_id = $1
		AND l.deleted_at IS NULL
		AND s.pipeline_stage NOT IN ('Completed', 'Lost')
)
SELECT lead_id, service_id, organization_id, stale_reason,
       pipeline_stage, status, last_activity_at,
       consumer_first_name, consumer_last_name, consumer_phone, consumer_email,
       service_type
FROM candidates
WHERE stale_reason IS NOT NULL
ORDER BY last_activity_at ASC NULLS FIRST
LIMIT $2
`

// ListStaleLeadServices returns stale lead services for an organization.
func (d *StaleLeadDetector) ListStaleLeadServices(ctx context.Context, organizationID uuid.UUID, limit int) ([]StaleLeadItem, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := d.pool.Query(ctx, staleLeadQuery, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("stale leads query: %w", err)
	}
	defer rows.Close()

	var items []StaleLeadItem
	for rows.Next() {
		var item StaleLeadItem
		var reasonStr *string
		err := rows.Scan(
			&item.LeadID,
			&item.ServiceID,
			&item.OrganizationID,
			&reasonStr,
			&item.PipelineStage,
			&item.Status,
			&item.LastActivityAt,
			&item.ConsumerFirstName,
			&item.ConsumerLastName,
			&item.ConsumerPhone,
			&item.ConsumerEmail,
			&item.ServiceType,
		)
		if err != nil {
			return nil, fmt.Errorf("stale leads scan: %w", err)
		}
		if reasonStr != nil {
			item.StaleReason = StaleReason(*reasonStr)
			items = append(items, item)
		}
	}

	return items, rows.Err()
}
