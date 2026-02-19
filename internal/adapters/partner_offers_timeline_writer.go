package adapters

import (
	"context"

	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
)

// PartnerOffersTimelineWriter adapts the leads TimelineEventStore for the partner offers domain.
type PartnerOffersTimelineWriter struct {
	store leadsrepo.TimelineEventStore
}

// NewPartnerOffersTimelineWriter creates a new timeline writer adapter.
func NewPartnerOffersTimelineWriter(store leadsrepo.TimelineEventStore) *PartnerOffersTimelineWriter {
	return &PartnerOffersTimelineWriter{store: store}
}

// WriteOfferEvent writes a timeline event from the partner-offer domain into the leads timeline.
func (a *PartnerOffersTimelineWriter) WriteOfferEvent(ctx context.Context, p notification.PartnerOfferTimelineEventParams) error {
	_, err := a.store.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		OrganizationID: p.OrgID,
		ActorType:      p.ActorType,
		ActorName:      p.ActorName,
		EventType:      p.EventType,
		Title:          p.Title,
		Summary:        p.Summary,
		Metadata:       p.Metadata,
	})
	return err
}

// Compile-time check.
var _ notification.PartnerOfferTimelineWriter = (*PartnerOffersTimelineWriter)(nil)
