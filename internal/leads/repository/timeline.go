package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TimelineSummaryMaxLen is the canonical maximum character length for timeline event summaries.
// Callers should use TruncateSummary when populating CreateTimelineEventParams.Summary.
const TimelineSummaryMaxLen = 400
const timelineDedupWindow = 90 * time.Second

// TruncateSummary trims text to maxLen, appending "..." on overflow.
// Returns nil for blank input.
func TruncateSummary(text string, maxLen int) *string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxLen {
		trimmed = trimmed[:maxLen] + "..."
	}
	return &trimmed
}

type TimelineEvent struct {
	ID             uuid.UUID
	LeadID         uuid.UUID
	ServiceID      *uuid.UUID
	OrganizationID uuid.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        *string
	Metadata       map[string]any
	CreatedAt      time.Time
}

type CreateTimelineEventParams struct {
	LeadID         uuid.UUID
	ServiceID      *uuid.UUID
	OrganizationID uuid.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        *string
	Metadata       map[string]any
}

func (r *Repository) CreateTimelineEvent(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, error) {
	if shouldAttemptTimelineDedup(params) {
		if existing, found, dedupeErr := r.findRecentDuplicateTimelineEvent(ctx, params); dedupeErr == nil && found {
			return existing, nil
		}
	}

	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return TimelineEvent{}, err
	}

	var event TimelineEvent
	var rawServiceID *uuid.UUID
	var summary *string

	// metadata is excluded from RETURNING: we already hold params.Metadata as a Go value.
	// Re-scanning the stored JSONB would add a redundant json.Unmarshal on every insert and
	// risks double-encoding if a caller ever passes pre-serialised data.
	err = r.pool.QueryRow(ctx, `
		INSERT INTO lead_timeline_events (
			lead_id,
			service_id,
			organization_id,
			actor_type,
			actor_name,
			event_type,
			title,
			summary,
			metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, created_at
	`, params.LeadID, params.ServiceID, params.OrganizationID, params.ActorType, params.ActorName, params.EventType, params.Title, params.Summary, metadataJSON).Scan(
		&event.ID,
		&event.LeadID,
		&rawServiceID,
		&event.OrganizationID,
		&event.ActorType,
		&event.ActorName,
		&event.EventType,
		&event.Title,
		&summary,
		&event.CreatedAt,
	)
	if err != nil {
		return TimelineEvent{}, err
	}

	if rawServiceID != nil {
		event.ServiceID = rawServiceID
	}
	if summary != nil {
		event.Summary = summary
	}
	// Assign directly from params — no JSON roundtrip needed.
	event.Metadata = params.Metadata

	return event, nil
}

func shouldAttemptTimelineDedup(params CreateTimelineEventParams) bool {
	if params.ActorType != ActorTypeAI && params.ActorType != ActorTypeSystem {
		return false
	}
	switch params.EventType {
	case EventTypeAI, EventTypeAnalysis, EventTypePhotoAnalysisCompleted, EventTypeStageChange:
		return true
	default:
		return false
	}
}

func metadataStringValue(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (r *Repository) findRecentDuplicateTimelineEvent(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, bool, error) {
	oldStage := metadataStringValue(params.Metadata, "oldStage")
	newStage := metadataStringValue(params.Metadata, "newStage")
	windowSeconds := int(timelineDedupWindow / time.Second)

	row := r.pool.QueryRow(ctx, `
		SELECT`+timelineSelectCols+`
		FROM lead_timeline_events
		WHERE lead_id = $1
			AND organization_id = $2
			AND (service_id = $3 OR (service_id IS NULL AND $3 IS NULL))
			AND actor_type = $4
			AND actor_name = $5
			AND event_type = $6
			AND title = $7
			AND (($8 IS NULL AND summary IS NULL) OR ($8 IS NOT NULL AND summary = $8))
			AND created_at >= now() - make_interval(secs => $9)
			AND (
				$6 <> $10 OR (
					COALESCE(metadata->>'oldStage', '') = $11
					AND COALESCE(metadata->>'newStage', '') = $12
				)
			)
		ORDER BY created_at DESC
		LIMIT 1
	`, params.LeadID, params.OrganizationID, params.ServiceID, params.ActorType, params.ActorName, params.EventType, params.Title, params.Summary, windowSeconds, EventTypeStageChange, oldStage, newStage)

	event, err := scanTimelineEvent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimelineEvent{}, false, nil
	}
	if err != nil {
		return TimelineEvent{}, false, err
	}
	return event, true, nil
}

// timelineRowScanner is satisfied by pgx.Rows and pgx.Row so that scanTimelineEvent
// can be shared between single-row and multi-row queries.
type timelineRowScanner interface {
	Scan(dest ...any) error
}

// scanTimelineEvent populates a TimelineEvent from a standard SELECT row.
// Column order must be: id, lead_id, service_id, organization_id,
// actor_type, actor_name, event_type, title, summary, metadata, created_at.
func scanTimelineEvent(s timelineRowScanner) (TimelineEvent, error) {
	var event TimelineEvent
	var rawServiceID *uuid.UUID
	var summary *string
	var rawMetadata []byte
	if err := s.Scan(
		&event.ID,
		&event.LeadID,
		&rawServiceID,
		&event.OrganizationID,
		&event.ActorType,
		&event.ActorName,
		&event.EventType,
		&event.Title,
		&summary,
		&rawMetadata,
		&event.CreatedAt,
	); err != nil {
		return TimelineEvent{}, err
	}
	if rawServiceID != nil {
		event.ServiceID = rawServiceID
	}
	if summary != nil {
		event.Summary = summary
	}
	if len(rawMetadata) > 0 {
		_ = json.Unmarshal(rawMetadata, &event.Metadata)
	}
	return event, nil
}

const timelineSelectCols = `
	id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, created_at`

// ListTimelineEvents returns all timeline events for a lead, ordered newest first.
// This includes both service-scoped events and lead-level events (service_id IS NULL).
func (r *Repository) ListTimelineEvents(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+timelineSelectCols+`
		FROM lead_timeline_events
		WHERE lead_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
	`, leadID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTimelineEvents(rows)
}

// ListTimelineEventsByService returns timeline events explicitly scoped to a specific
// service (service_id = serviceID). Events with a NULL service_id are intentionally
// excluded — they represent lead-level activity visible only in ListTimelineEvents.
//
// This prevents general notes/updates from polluting the per-service history in
// multi-service leads (e.g., a "Manual Update" note must not appear in both the
// Solar and Windows service timelines just because it has no service_id attached).
func (r *Repository) ListTimelineEventsByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+timelineSelectCols+`
		FROM lead_timeline_events
		WHERE lead_id = $1 AND organization_id = $2 AND service_id = $3
		ORDER BY created_at DESC
	`, leadID, organizationID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTimelineEvents(rows)
}

// collectTimelineEvents drains pgx rows into a slice of TimelineEvent.
func collectTimelineEvents(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]TimelineEvent, error) {
	items := make([]TimelineEvent, 0)
	for rows.Next() {
		event, err := scanTimelineEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
