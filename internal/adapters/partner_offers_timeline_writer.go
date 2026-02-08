package adapters

import (
	"context"

	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"

	"github.com/google/uuid"
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
func (a *PartnerOffersTimelineWriter) WriteOfferEvent(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, orgID uuid.UUID, actorType, actorName, eventType, title string, summary *string, metadata map[string]any) error {
	_, err := a.store.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      serviceID,
		OrganizationID: orgID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      eventType,
		Title:          title,
		Summary:        summary,
		Metadata:       metadata,
	})
	return err
}

// Compile-time check.
var _ notification.PartnerOfferTimelineWriter = (*PartnerOffersTimelineWriter)(nil)
