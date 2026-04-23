package leads

import (
	"context"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

func (o *Orchestrator) recordDispatcherFailure(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) {
	summary := "Partner matching mislukt. Probeer opnieuw."
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameDispatcher,
		EventType:      repository.EventTypeAlert,
		Title:          repository.EventTitleDispatcherFailed,
		Summary:        &summary,
		Metadata: repository.AlertMetadata{
			Trigger: "dispatcher_run",
		}.ToMap(),
	})
}
type maybeWriteTimelineParams struct {
	LeadID       uuid.UUID
	ServiceID    uuid.UUID
	TenantID     uuid.UUID
	TriggerEvent string
	OldStage     string
	NewStage     string
	OldStatus    string
	NewStatus    string
	ReasonCode   string
	Reason       string
	Resurrecting bool
	Aggregates   repository.ServiceStateAggregates
}
func (o *Orchestrator) maybeWriteReconcileTimeline(ctx context.Context, p maybeWriteTimelineParams) {
	isRegression := isPipelineRegression(p.OldStage, p.NewStage)
	if !isRegression && !p.Resurrecting && p.ReasonCode != "stale_draft_decay" {
		return
	}
	summary := p.Reason
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         p.LeadID,
		ServiceID:      &p.ServiceID,
		OrganizationID: p.TenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameStateReconciler,
		EventType:      repository.EventTypeStateReconciled,
		Title:          repository.EventTitleStateReconciled,
		Summary:        &summary,
		Metadata: repository.StateReconciledMetadata{
			ReasonCode: p.ReasonCode,
			Trigger:    p.TriggerEvent,
			OldStage:   p.OldStage,
			NewStage:   p.NewStage,
			OldStatus:  p.OldStatus,
			NewStatus:  p.NewStatus,
			Evidence:   buildReconcileEvidence(p.Aggregates),
		}.ToMap(),
		Visibility: repository.TimelineVisibilityDebug,
	})
}
