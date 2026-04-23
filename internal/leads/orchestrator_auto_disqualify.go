package leads

import (
	"context"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
)

func (o *Orchestrator) maybeAutoDisqualifyJunk(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) {
	settings, err := o.loadOrgAISettings(ctx, tenantID)
	if err != nil {
		o.log.Warn("orchestrator: skipping junk auto-disqualify (settings load failed)", "tenantId", tenantID, "error", err)
		return
	}
	if !settings.AIAutoDisqualifyJunk {
		return
	}

	svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for junk auto-disqualify", "error", err)
		return
	}
	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		return
	}
	if svc.Status == domain.LeadStatusDisqualified {
		return
	}

	analysis, err := o.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		if err == repository.ErrNotFound {
			return
		}
		o.log.Error("orchestrator: failed to fetch latest AI analysis for junk check", "error", err)
		return
	}
	if analysis.LeadQuality != "Junk" {
		return
	}

	o.log.Info("orchestrator: auto-disqualifying Junk lead", "leadId", leadID, "serviceId", serviceID)

	if _, _, err := o.updateServiceState(ctx, serviceID, tenantID, svc.Status, svc.PipelineStage, domain.LeadStatusDisqualified, domain.PipelineStageLost); err != nil {
		o.log.Error("orchestrator: failed to auto-disqualify Junk lead", "error", err)
		return
	}

	summary := "AI detected Junk quality. Lead automatically moved to Disqualified."
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      repository.ActorTypeAI,
		ActorName:      repository.ActorNameGatekeeper,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleAutoDisqualified,
		Summary:        &summary,
		Metadata: repository.AutoDisqualifyMetadata{
			LeadQuality:       analysis.LeadQuality,
			RecommendedAction: analysis.RecommendedAction,
			AnalysisID:        analysis.ID,
			Reason:            "junk_quality",
		}.ToMap(),
	})

	o.eventBus.Publish(ctx, events.LeadAutoDisqualified{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		TenantID:      tenantID,
		Reason:        "junk_quality",
	})
}

func (o *Orchestrator) OnLeadAutoDisqualified(ctx context.Context, evt events.LeadAutoDisqualified) {
	o.cancelPendingWorkflows(ctx, evt.TenantID, evt.LeadID, "lead_auto_disqualified")

	o.sse.PublishToOrganization(evt.TenantID, sse.Event{
		Type:      sse.EventLeadStatusChanged,
		LeadID:    evt.LeadID,
		ServiceID: evt.LeadServiceID,
		Message:   "Lead automatisch gedisqualificeerd",
		Data: map[string]any{
			"reason": evt.Reason,
		},
	})
}
