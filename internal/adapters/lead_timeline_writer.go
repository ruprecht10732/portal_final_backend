package adapters

import (
	"context"

	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
)

// LeadTimelineWriter adapts the leads TimelineEventStore for generic lead timeline events.
type LeadTimelineWriter struct {
	store leadsrepo.TimelineEventStore
}

// NewLeadTimelineWriter creates a new lead timeline writer adapter.
func NewLeadTimelineWriter(store leadsrepo.TimelineEventStore) *LeadTimelineWriter {
	return &LeadTimelineWriter{store: store}
}

// CreateTimelineEvent writes a lead timeline event.
func (a *LeadTimelineWriter) CreateTimelineEvent(ctx context.Context, params notification.LeadTimelineEventParams) error {
	_, err := a.store.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         params.LeadID,
		ServiceID:      params.ServiceID,
		OrganizationID: params.OrgID,
		ActorType:      params.ActorType,
		ActorName:      params.ActorName,
		EventType:      params.EventType,
		Title:          params.Title,
		Summary:        params.Summary,
		Metadata:       params.Metadata,
	})
	return err
}

// Compile-time check.
var _ notification.LeadTimelineWriter = (*LeadTimelineWriter)(nil)
