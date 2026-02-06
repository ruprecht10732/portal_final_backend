package adapters

import (
	"context"

	leadsrepo "portal_final_backend/internal/leads/repository"
	quotesvc "portal_final_backend/internal/quotes/service"
)

// QuotesTimelineWriter adapts the leads TimelineEventStore for the quotes domain.
// It implements quotes/service.TimelineWriter using interface-segregation.
type QuotesTimelineWriter struct {
	store leadsrepo.TimelineEventStore
}

// NewQuotesTimelineWriter creates a new timeline writer adapter.
func NewQuotesTimelineWriter(store leadsrepo.TimelineEventStore) *QuotesTimelineWriter {
	return &QuotesTimelineWriter{store: store}
}

// CreateTimelineEvent writes a timeline event from the quotes domain into the leads timeline.
func (a *QuotesTimelineWriter) CreateTimelineEvent(ctx context.Context, params quotesvc.TimelineEventParams) error {
	_, err := a.store.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         params.LeadID,
		ServiceID:      params.ServiceID,
		OrganizationID: params.OrganizationID,
		ActorType:      params.ActorType,
		ActorName:      params.ActorName,
		EventType:      params.EventType,
		Title:          params.Title,
		Summary:        params.Summary,
		Metadata:       params.Metadata,
	})
	return err
}

// Compile-time check that QuotesTimelineWriter implements quotes/service.TimelineWriter.
var _ quotesvc.TimelineWriter = (*QuotesTimelineWriter)(nil)
