package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

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
	metadataJSON, err := json.Marshal(params.Metadata)
	if err != nil {
		return TimelineEvent{}, err
	}

	var event TimelineEvent
	var rawServiceID *uuid.UUID
	var summary *string
	var rawMetadata []byte

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
		RETURNING id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, created_at
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
		&rawMetadata,
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
	if len(rawMetadata) > 0 {
		_ = json.Unmarshal(rawMetadata, &event.Metadata)
	}

	return event, nil
}

func (r *Repository) ListTimelineEvents(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, created_at
		FROM lead_timeline_events
		WHERE lead_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
	`, leadID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TimelineEvent, 0)
	for rows.Next() {
		var event TimelineEvent
		var rawServiceID *uuid.UUID
		var summary *string
		var rawMetadata []byte
		if err := rows.Scan(
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
			return nil, err
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
		items = append(items, event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}

func (r *Repository) ListTimelineEventsByService(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, created_at
		FROM lead_timeline_events
		WHERE lead_id = $1 AND organization_id = $2 AND (service_id = $3 OR service_id IS NULL)
		ORDER BY created_at DESC
	`, leadID, organizationID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TimelineEvent, 0)
	for rows.Next() {
		var event TimelineEvent
		var rawServiceID *uuid.UUID
		var summary *string
		var rawMetadata []byte
		if err := rows.Scan(
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
			return nil, err
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
		items = append(items, event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}
