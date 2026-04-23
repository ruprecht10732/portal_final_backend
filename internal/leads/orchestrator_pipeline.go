package leads

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/scheduler"
)

func (o *Orchestrator) OnVisitReportSubmitted(ctx context.Context, evt events.VisitReportSubmitted) {
	if o.auditor == nil {
		return
	}

	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for visit report", "error", err)
		return
	}
	if !o.ShouldRunAgent(svc) {
		return
	}

	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for visit report audit", "serviceId", evt.LeadServiceID)
		return
	}
	if err := o.automationQueue.EnqueueAuditVisitReport(ctx, scheduler.AuditVisitReportPayload{
		TenantID:      evt.TenantID.String(),
		LeadID:        evt.LeadID.String(),
		LeadServiceID: evt.LeadServiceID.String(),
		AppointmentID: evt.AppointmentID.String(),
	}); err != nil {
		o.log.Error("orchestrator: failed to enqueue visit report audit", "error", err, "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) OnStageChange(ctx context.Context, evt events.PipelineStageChanged) {
	// Terminal stages never trigger agents
	if domain.IsTerminalPipelineStage(evt.NewStage) {
		o.log.Info(orchestratorAutomationLog, "agent", "stage-router", "decision", "skip", "reason", "terminal_stage", "serviceId", evt.LeadServiceID, "newStage", evt.NewStage)
		return
	}
	if o.shouldSkipDuplicateStageEvent(evt) {
		o.log.Info("orchestrator: duplicate stage event skipped", "serviceId", evt.LeadServiceID, "oldStage", evt.OldStage, "newStage", evt.NewStage)
		return
	}
	// Intentionally no generic stage-change timeline write here.
	// Agent tools already persist detailed stage-change reasons.

	switch evt.NewStage {
	case domain.PipelineStageEstimation:
		o.handleEstimationStage(evt)
	case domain.PipelineStageFulfillment:
		o.handleFulfillmentStage(evt)
	case domain.PipelineStageManualIntervention:
		o.handleManualInterventionStage(evt)
	}
}

func (o *Orchestrator) handleEstimationStage(evt events.PipelineStageChanged) {
	settings, err := o.loadOrgAISettings(context.Background(), evt.TenantID)
	if err != nil {
		o.log.Warn("orchestrator: skipping estimator (settings load failed)", "tenantId", evt.TenantID, "error", err)
		return
	}
	if !settings.AIAutoEstimate {
		o.log.Info(orchestratorAutomationLog, "agent", "calculator", "decision", "skip", "reason", "auto_estimate_disabled", "tenantId", evt.TenantID, "serviceId", evt.LeadServiceID)
		return
	}

	o.log.Info(orchestratorAutomationLog, "agent", "calculator", "decision", "enqueue", "reason", "stage_estimation", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for estimator", "serviceId", evt.LeadServiceID)
		return
	}
	if !maybeEnqueueEstimatorRun(estimatorEnqueueRequest{
		ctx:       context.Background(),
		repo:      o.repo,
		deduper:   o.estimatorDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.TenantID,
		force:     false,
		source:    "pipeline_stage_change",
	}) {
		o.log.Error("orchestrator: estimator failed to enqueue", "serviceId", evt.LeadServiceID)
	}
}

func (o *Orchestrator) handleFulfillmentStage(evt events.PipelineStageChanged) {
	settings, err := o.loadOrgAISettings(context.Background(), evt.TenantID)
	if err != nil {
		o.log.Warn("orchestrator: skipping dispatcher (settings load failed)", "tenantId", evt.TenantID, "error", err)
		return
	}
	if !settings.AIAutoDispatch {
		o.log.Info(orchestratorAutomationLog, "agent", "matchmaker", "decision", "skip", "reason", "auto_dispatch_disabled", "tenantId", evt.TenantID, "serviceId", evt.LeadServiceID)
		return
	}

	o.log.Info(orchestratorAutomationLog, "agent", "matchmaker", "decision", "enqueue", "reason", "stage_fulfillment", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for dispatcher", "serviceId", evt.LeadServiceID)
		return
	}
	if !maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{
		ctx:       context.Background(),
		repo:      o.repo,
		deduper:   o.dispatcherDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.TenantID,
		source:    "pipeline_stage_change",
	}) {
		o.log.Error("orchestrator: dispatcher failed to enqueue", "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) handleManualInterventionStage(evt events.PipelineStageChanged) {
	o.log.Warn("orchestrator: manual intervention required", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
	summary := "Geautomatiseerde verwerking vereist menselijke beoordeling"
	title := repository.EventTitleManualIntervention
	actorName := repository.ActorNameOrchestrator
	reasonCode := strings.TrimSpace(evt.ReasonCode)
	if reasonCode == "" {
		reasonCode = strings.TrimSpace(evt.Trigger)
	}
	if reasonCode == "" {
		reasonCode = "manual_intervention_required"
	}
	metadata := repository.ManualInterventionMetadata{
		RunID:          evt.RunID,
		PreviousStage:  evt.OldStage,
		Trigger:        "pipeline_stage_change",
		ReasonCategory: evt.Trigger,
		ReasonCode:     reasonCode,
		Drafts:         buildManualInterventionDrafts(evt.LeadID, evt.LeadServiceID),
	}
	if strings.TrimSpace(evt.Reason) != "" {
		summary = strings.TrimSpace(evt.Reason)
		metadata.ReasonSummary = summary
	}
	if evt.Trigger == "ai_loop_detected" {
		title = repository.EventTitleAILoopDetected
		actorName = repository.ActorNameLoopDetector
		metadata.Trigger = evt.Trigger
		metadata.ReasonCategory = evt.Trigger
		metadata.ReasonCode = "nurturing_loop_threshold"
		if svc, err := o.repo.GetLeadServiceByID(context.Background(), evt.LeadServiceID, evt.TenantID); err == nil {
			metadata.AttemptCount = svc.GatekeeperNurturingLoopCount
			if svc.GatekeeperNurturingLoopFingerprint != nil {
				metadata.BlockerFingerprint = *svc.GatekeeperNurturingLoopFingerprint
			}
		}
	}
	// Record timeline event for audit trail
	_, _ = o.repo.CreateTimelineEvent(context.Background(), repository.CreateTimelineEventParams{
		LeadID:         evt.LeadID,
		ServiceID:      &evt.LeadServiceID,
		OrganizationID: evt.TenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      actorName,
		EventType:      repository.EventTypeAlert,
		Title:          title,
		Summary:        &summary,
		Metadata:       metadata.ToMap(),
	})

	// Push real-time notification to all org members via SSE
	o.sse.PublishToOrganization(evt.TenantID, sse.Event{
		Type:      sse.EventManualIntervention,
		LeadID:    evt.LeadID,
		ServiceID: evt.LeadServiceID,
		Message:   summary,
		Data: map[string]any{
			"previousStage": evt.OldStage,
			"trigger":       metadata.Trigger,
		},
	})

	// Publish domain event for downstream handlers (email notifications, etc.)
	o.eventBus.Publish(context.Background(), events.ManualInterventionRequired{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: evt.LeadServiceID,
		TenantID:      evt.TenantID,
		Reason:        summary,
		ReasonCode:    metadata.ReasonCode,
		Context:       "Transitioned from " + evt.OldStage,
		RunID:         evt.RunID,
	})
}
func (o *Orchestrator) OnQuoteAccepted(ctx context.Context, evt events.QuoteAccepted) {
	if evt.LeadServiceID == nil {
		return
	}
	serviceID := *evt.LeadServiceID

	oldStatus := ""
	oldStage := ""
	if svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, evt.OrganizationID); err == nil {
		oldStatus = svc.Status
		oldStage = svc.PipelineStage
	}

	summary := fmt.Sprintf("Offerte %s geaccepteerd. Starten met zoeken naar partner.", evt.QuoteNumber)
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         evt.LeadID,
		ServiceID:      evt.LeadServiceID,
		OrganizationID: evt.OrganizationID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameOrchestrator,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleQuoteAccepted,
		Summary:        &summary,
		Metadata: repository.QuoteEventMetadata{
			QuoteID: evt.QuoteID,
		}.ToMap(),
	})

	if _, _, err := o.updateServiceState(ctx, serviceID, evt.OrganizationID, oldStatus, oldStage, domain.LeadStatusInProgress, domain.PipelineStageFulfillment); err != nil {
		o.log.Error("orchestrator: failed to advance state after quote acceptance", "error", err)
		return
	}

	if evt.TotalCents > 0 {
		if err := o.repo.UpdateProjectedValueCents(ctx, evt.LeadID, evt.OrganizationID, evt.TotalCents); err != nil {
			o.log.Error("orchestrator: failed to update lead projected value after quote acceptance", "error", err)
		}
	}

	o.cancelPendingWorkflows(ctx, evt.OrganizationID, evt.LeadID, "quote_accepted")

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      domain.PipelineStageFulfillment,
	})
}
func (o *Orchestrator) OnQuoteRejected(ctx context.Context, evt events.QuoteRejected) {
	if evt.LeadServiceID == nil {
		return
	}

	serviceID := *evt.LeadServiceID
	oldStatus := ""
	oldStage := ""
	if svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, evt.OrganizationID); err == nil {
		oldStatus = svc.Status
		oldStage = svc.PipelineStage
	}

	reason := strings.TrimSpace(evt.Reason)
	summary := fmt.Sprintf("Offerte afgewezen. Reden: %s", reason)
	if reason == "" {
		summary = "Offerte afgewezen. Pipeline gemarkeerd als verloren."
	}

	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         evt.LeadID,
		ServiceID:      evt.LeadServiceID,
		OrganizationID: evt.OrganizationID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameOrchestrator,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleQuoteRejected,
		Summary:        &summary,
		Metadata: repository.QuoteEventMetadata{
			QuoteID: evt.QuoteID,
			Reason:  evt.Reason,
		}.ToMap(),
	})

	if _, _, err := o.updateServiceState(ctx, serviceID, evt.OrganizationID, oldStatus, oldStage, domain.LeadStatusDisqualified, domain.PipelineStageLost); err != nil {
		o.log.Error("orchestrator: failed to advance state after quote rejection", "error", err)
		return
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      domain.PipelineStageLost,
	})
}
func (o *Orchestrator) OnQuoteSent(ctx context.Context, evt events.QuoteSent) {
	if evt.LeadServiceID == nil {
		return
	}

	serviceID := *evt.LeadServiceID
	svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, evt.OrganizationID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for quote sent", "error", err)
		return
	}

	if svc.PipelineStage == domain.PipelineStageProposal {
		return
	}

	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		return
	}

	oldStage := svc.PipelineStage
	if _, _, err := o.updateServiceState(ctx, serviceID, evt.OrganizationID, svc.Status, svc.PipelineStage, domain.LeadStatusPending, domain.PipelineStageProposal); err != nil {
		o.log.Error("orchestrator: failed to advance state after quote sent", "error", err)
		return
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      domain.PipelineStageProposal,
	})
}
func (o *Orchestrator) OnPartnerOfferRejected(ctx context.Context, evt events.PartnerOfferRejected) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
	o.log.Info("Orchestrator: Partner rejected offer, re-triggering dispatcher", "leadId", evt.LeadID)
	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured after partner rejection", "serviceId", evt.LeadServiceID)
		return
	}
	if !maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{
		ctx:       ctx,
		repo:      o.repo,
		deduper:   o.dispatcherDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.OrganizationID,
		source:    "partner_offer_rejected",
	}) {
		o.log.Error("orchestrator: failed to enqueue dispatcher after rejection", "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) OnPartnerOfferExpired(ctx context.Context, evt events.PartnerOfferExpired) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
	o.log.Info("Orchestrator: Partner offer expired, re-triggering dispatcher", "leadId", evt.LeadID)
	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured after partner offer expiry", "serviceId", evt.LeadServiceID)
		return
	}
	if !maybeEnqueueDispatcherRun(dispatcherEnqueueRequest{
		ctx:       ctx,
		repo:      o.repo,
		deduper:   o.dispatcherDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.OrganizationID,
		source:    "partner_offer_expired",
	}) {
		o.log.Error("orchestrator: failed to enqueue dispatcher after expiry", "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) OnPartnerOfferAccepted(ctx context.Context, evt events.PartnerOfferAccepted) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}
func (o *Orchestrator) OnPartnerOfferCreated(ctx context.Context, evt events.PartnerOfferCreated) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}
func (o *Orchestrator) OnPartnerOfferDeleted(ctx context.Context, evt events.PartnerOfferDeleted) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
}
func buildManualInterventionDrafts(leadID, serviceID uuid.UUID) map[string]any {
	subject := "Handmatige interventie vereist"
	body := fmt.Sprintf("Er is handmatige interventie vereist voor lead %s (service %s).\n\nControleer de intake, ontbrekende gegevens en eventuele AI-analyses en bepaal de volgende stap.", leadID.String(), serviceID.String())
	whatsApp := fmt.Sprintf("Handmatige interventie vereist voor lead %s (service %s). Controleer de intake en bepaal de volgende stap.", leadID.String(), serviceID.String())

	return map[string]any{
		"emailSubject":    subject,
		"emailBody":       body,
		"whatsappMessage": whatsApp,
		"messageLanguage": "nl",
		"messageAudience": "internal",
		"messageCategory": "manual_intervention",
	}
}
