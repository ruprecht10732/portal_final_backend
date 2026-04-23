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
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
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
	Visibility     string
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
	Visibility     string
}

func normalizeTimelineVisibility(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case TimelineVisibilityInternal:
		return TimelineVisibilityInternal
	case TimelineVisibilityDebug:
		return TimelineVisibilityDebug
	default:
		return TimelineVisibilityPublic
	}
}

func (r *Repository) CreateTimelineEvent(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, error) {
	params.Visibility = normalizeTimelineVisibility(params.Visibility)
	if shouldAttemptTimelineDedup(params) {
		var existing TimelineEvent
		var found bool
		var dedupeErr error
		if params.EventType == EventTypeAlert {
			// Relaxed dedup for alerts: match on (lead, service, event_type, title)
			// regardless of actor_name or summary, so Orchestrator and LoopDetector
			// alerts for the same service are deduplicated.
			existing, found, dedupeErr = r.findRecentDuplicateAlertByTitle(ctx, params)
		} else {
			existing, found, dedupeErr = r.findRecentDuplicateTimelineEvent(ctx, params)
		}
		if dedupeErr == nil && found {
			return existing, nil
		}
	}

	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return TimelineEvent{}, err
	}

	row, err := r.queries.CreateTimelineEvent(ctx, leadsdb.CreateTimelineEventParams{
		LeadID:         toPgUUID(params.LeadID),
		ServiceID:      toPgUUIDPtr(params.ServiceID),
		OrganizationID: toPgUUID(params.OrganizationID),
		ActorType:      params.ActorType,
		ActorName:      params.ActorName,
		EventType:      params.EventType,
		Title:          params.Title,
		Summary:        toPgText(params.Summary),
		Metadata:       metadataJSON,
		Visibility:     params.Visibility,
	})
	if err != nil {
		return TimelineEvent{}, err
	}

	event := timelineEventFromCreateRow(row)
	// Preserve the original Go map and avoid a needless JSON roundtrip on insert.
	event.Metadata = params.Metadata
	return event, nil
}

func shouldAttemptTimelineDedup(params CreateTimelineEventParams) bool {
	if params.ActorType != ActorTypeAI && params.ActorType != ActorTypeSystem {
		return false
	}
	switch params.EventType {
	case EventTypeAI, EventTypeAnalysis, EventTypePhotoAnalysisCompleted, EventTypeStageChange, EventTypeAlert:
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

func (r *Repository) findRecentDuplicateAlertByTitle(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, bool, error) {
	windowSeconds := int(timelineDedupWindow / time.Second)
	row, err := r.queries.FindRecentDuplicateAlertByTitle(ctx, leadsdb.FindRecentDuplicateAlertByTitleParams{
		LeadID:         toPgUUID(params.LeadID),
		OrganizationID: toPgUUID(params.OrganizationID),
		ServiceID:      toPgUUIDPtr(params.ServiceID),
		EventType:      params.EventType,
		Title:          params.Title,
		Secs:           float64(windowSeconds),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TimelineEvent{}, false, nil
	}
	if err != nil {
		return TimelineEvent{}, false, err
	}
	return timelineEventFromAlertDuplicateRow(row), true, nil
}

func (r *Repository) findRecentDuplicateTimelineEvent(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, bool, error) {
	oldStage := metadataStringValue(params.Metadata, "oldStage")
	newStage := metadataStringValue(params.Metadata, "newStage")
	windowSeconds := int(timelineDedupWindow / time.Second)

	row, err := r.queries.FindRecentDuplicateTimelineEvent(ctx, leadsdb.FindRecentDuplicateTimelineEventParams{
		LeadID:         toPgUUID(params.LeadID),
		OrganizationID: toPgUUID(params.OrganizationID),
		ServiceID:      toPgUUIDPtr(params.ServiceID),
		ActorType:      params.ActorType,
		ActorName:      params.ActorName,
		EventType:      params.EventType,
		Title:          params.Title,
		Column8:        params.Summary,
		Visibility:     params.Visibility,
		Secs:           float64(windowSeconds),
		Column11:       EventTypeStageChange,
		Column12:       oldStage,
		Column13:       newStage,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TimelineEvent{}, false, nil
	}
	if err != nil {
		return TimelineEvent{}, false, err
	}
	return timelineEventFromDuplicateRow(row), true, nil
}

// ListTimelineEvents returns all timeline events for a lead, ordered newest first.
// This includes both service-scoped events and lead-level events (service_id IS NULL).
func (r *Repository) ListTimelineEvents(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.queries.ListTimelineEvents(ctx, leadsdb.ListTimelineEventsParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, err
	}
	items := make([]TimelineEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, timelineEventFromListRow(row))
	}
	return items, nil
}

// ListTimelineEventsByService returns timeline events explicitly scoped to a specific
// service (service_id = serviceID). Events with a NULL service_id are intentionally
// excluded — they represent lead-level activity visible only in ListTimelineEvents.
//
// This prevents general notes/updates from polluting the per-service history in
// multi-service leads (e.g., a "Manual Update" note must not appear in both the
// Solar and Windows service timelines just because it has no service_id attached).
func (r *Repository) ListTimelineEventsByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.queries.ListTimelineEventsByService(ctx, leadsdb.ListTimelineEventsByServiceParams{
		LeadID:         toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
		ServiceID:      toPgUUID(serviceID),
	})
	if err != nil {
		return nil, err
	}
	items := make([]TimelineEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, timelineEventFromListByServiceRow(row))
	}
	return items, nil
}

type timelineEventSourceParams struct {
	ID             pgtype.UUID
	LeadID         pgtype.UUID
	ServiceID      pgtype.UUID
	OrganizationID pgtype.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        pgtype.Text
	Metadata       []byte
	Visibility     string
	CreatedAt      pgtype.Timestamptz
}

func timelineEventFromSource(p timelineEventSourceParams) TimelineEvent {
	event := TimelineEvent{
		ID:             p.ID.Bytes,
		LeadID:         p.LeadID.Bytes,
		ServiceID:      optionalUUID(p.ServiceID),
		OrganizationID: p.OrganizationID.Bytes,
		ActorType:      p.ActorType,
		ActorName:      p.ActorName,
		EventType:      p.EventType,
		Title:          p.Title,
		Summary:        optionalString(p.Summary),
		Visibility:     normalizeTimelineVisibility(p.Visibility),
		CreatedAt:      p.CreatedAt.Time,
	}
	if len(p.Metadata) > 0 {
		_ = json.Unmarshal(p.Metadata, &event.Metadata)
	}
	return event
}

func timelineEventFromCreateRow(row leadsdb.CreateTimelineEventRow) TimelineEvent {
	return timelineEventFromSource(timelineEventSourceParams{
		ID: row.ID, LeadID: row.LeadID, ServiceID: row.ServiceID, OrganizationID: row.OrganizationID,
		ActorType: row.ActorType, ActorName: row.ActorName, EventType: row.EventType, Title: row.Title,
		Summary: row.Summary, Metadata: row.Metadata, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	})
}

func timelineEventFromDuplicateRow(row leadsdb.FindRecentDuplicateTimelineEventRow) TimelineEvent {
	return timelineEventFromSource(timelineEventSourceParams{
		ID: row.ID, LeadID: row.LeadID, ServiceID: row.ServiceID, OrganizationID: row.OrganizationID,
		ActorType: row.ActorType, ActorName: row.ActorName, EventType: row.EventType, Title: row.Title,
		Summary: row.Summary, Metadata: row.Metadata, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	})
}

func timelineEventFromAlertDuplicateRow(row leadsdb.FindRecentDuplicateAlertByTitleRow) TimelineEvent {
	return timelineEventFromSource(timelineEventSourceParams{
		ID: row.ID, LeadID: row.LeadID, ServiceID: row.ServiceID, OrganizationID: row.OrganizationID,
		ActorType: row.ActorType, ActorName: row.ActorName, EventType: row.EventType, Title: row.Title,
		Summary: row.Summary, Metadata: row.Metadata, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	})
}

func timelineEventFromListRow(row leadsdb.ListTimelineEventsRow) TimelineEvent {
	return timelineEventFromSource(timelineEventSourceParams{
		ID: row.ID, LeadID: row.LeadID, ServiceID: row.ServiceID, OrganizationID: row.OrganizationID,
		ActorType: row.ActorType, ActorName: row.ActorName, EventType: row.EventType, Title: row.Title,
		Summary: row.Summary, Metadata: row.Metadata, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	})
}

func timelineEventFromListByServiceRow(row leadsdb.ListTimelineEventsByServiceRow) TimelineEvent {
	return timelineEventFromSource(timelineEventSourceParams{
		ID: row.ID, LeadID: row.LeadID, ServiceID: row.ServiceID, OrganizationID: row.OrganizationID,
		ActorType: row.ActorType, ActorName: row.ActorName, EventType: row.EventType, Title: row.Title,
		Summary: row.Summary, Metadata: row.Metadata, Visibility: row.Visibility, CreatedAt: row.CreatedAt,
	})
}
