package leads

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/scheduler"
)

func (o *Orchestrator) canRunGatekeeperForPhotoEvent(ctx context.Context, evt events.PhotoAnalysisCompleted) bool {
	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for photo analysis event", "error", err)
		return false
	}

	if !o.ShouldRunAgent(svc) {
		return false
	}

	if !domain.AllowsGatekeeperEvaluation(svc.PipelineStage) {
		return false
	}

	return true
}
func (o *Orchestrator) OnDataChange(ctx context.Context, evt events.LeadDataChanged) {
	o.log.Info("orchestrator: data change received", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "source", evt.Source, "tenantId", evt.TenantID)
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.TenantID, evt.EventName(), evt.OccurredAt(), false)

	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service", "error", err)
		return
	}

	o.maybeRunAuditorForCallLog(evt)

	if !o.ShouldRunAgent(svc) {
		return
	}

	o.maybeReRunEstimatorForNote(svc, evt)
	o.maybeRunGatekeeperForDataChange(svc, evt)
}

func (o *Orchestrator) maybeRunAuditorForCallLog(evt events.LeadDataChanged) {
	if o.auditor == nil || !strings.EqualFold(evt.Source, "call_log") {
		return
	}
	o.log.Info(orchestratorAutomationLog, "agent", "auditor", "decision", "evaluate", "reason", "call_log_source", "serviceId", evt.LeadServiceID, "leadId", evt.LeadID)
	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for call log audit", "serviceId", evt.LeadServiceID)
		return
	}
	if err := o.automationQueue.EnqueueAuditCallLog(context.Background(), scheduler.AuditCallLogPayload{
		TenantID:      evt.TenantID.String(),
		LeadID:        evt.LeadID.String(),
		LeadServiceID: evt.LeadServiceID.String(),
	}); err != nil {
		o.log.Error("orchestrator: failed to enqueue call log audit", "error", err, "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) maybeReRunEstimatorForNote(svc repository.LeadService, evt events.LeadDataChanged) {
	if !strings.EqualFold(strings.TrimSpace(evt.Source), "note") {
		return
	}
	if svc.PipelineStage != domain.PipelineStageEstimation && svc.PipelineStage != domain.PipelineStageManualIntervention {
		return
	}

	settings, err := o.loadOrgAISettings(context.Background(), evt.TenantID)
	if err != nil {
		o.log.Warn("orchestrator: skipping estimator re-run for note (settings load failed)", "tenantId", evt.TenantID, "error", err)
		return
	}
	if !settings.AIAutoEstimate {
		o.log.Info(orchestratorAutomationLog, "agent", "calculator", "decision", "skip", "reason", "auto_estimate_disabled", "tenantId", evt.TenantID, "serviceId", evt.LeadServiceID)
		return
	}

	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for estimator re-run", "serviceId", evt.LeadServiceID)
		return
	}

	o.log.Info(orchestratorAutomationLog, "agent", "calculator", "decision", "enqueue", "reason", "note_added_in_estimation", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "stage", svc.PipelineStage)
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
		source:    "note_in_estimation",
	}) {
		o.log.Error("orchestrator: estimator re-run failed to enqueue", "serviceId", evt.LeadServiceID)
	}
}
func (o *Orchestrator) maybeRunGatekeeperForDataChange(svc repository.LeadService, evt events.LeadDataChanged) {
	if svc.PipelineStage == domain.PipelineStageManualIntervention {
		o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "skip", "reason", "manual_intervention_active", "serviceId", evt.LeadServiceID, "leadId", evt.LeadID)
		return
	}

	if !domain.AllowsGatekeeperEvaluation(svc.PipelineStage) {
		o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "skip", "reason", "stage_not_eligible", "serviceId", evt.LeadServiceID, "leadId", evt.LeadID, "stage", svc.PipelineStage)
		return
	}
	if strings.EqualFold(strings.TrimSpace(evt.Source), "customer_portal_upload") && o.serviceHasImageAttachments(context.Background(), evt.LeadServiceID, evt.TenantID) {
		o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "defer", "reason", "photo_analysis_pending", "serviceId", evt.LeadServiceID, "leadId", evt.LeadID, "source", evt.Source)
		return
	}

	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured for gatekeeper", "serviceId", evt.LeadServiceID)
		return
	}
	o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "enqueue", "reason", "data_change", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "stage", svc.PipelineStage, "source", evt.Source)
	_ = maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{
		ctx:       context.Background(),
		repo:      o.repo,
		deduper:   o.gatekeeperDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.TenantID,
		source:    evt.Source,
	})
}
func (o *Orchestrator) serviceHasImageAttachments(ctx context.Context, serviceID, tenantID uuid.UUID) bool {
	attachments, err := o.repo.ListAttachmentsByService(ctx, serviceID, tenantID)
	if err != nil {
		return false
	}
	now := time.Now()
	const photoAnalysisGatekeeperBlockTimeout = 5 * time.Minute
	for _, att := range attachments {
		if att.ContentType != nil && isImageContentType(*att.ContentType) {
			if now.Sub(att.CreatedAt) > photoAnalysisGatekeeperBlockTimeout {
				o.log.Warn("orchestrator: image attachment has been unanalyzed for too long; allowing gatekeeper to proceed", "serviceId", serviceID, "attachmentId", att.ID, "createdAt", att.CreatedAt)
				continue
			}
			return true
		}
	}
	return false
}
func (o *Orchestrator) OnPhotoAnalysisCompleted(ctx context.Context, evt events.PhotoAnalysisCompleted) {
	if !o.canRunGatekeeperForPhotoEvent(ctx, evt) {
		o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "skip", "reason", "photo_event_not_eligible", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
		return
	}

	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured after photo analysis", "serviceId", evt.LeadServiceID)
		return
	}
	o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "enqueue", "reason", "photo_analysis_complete", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "summary", evt.Summary)
	_ = maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{
		ctx:       ctx,
		repo:      o.repo,
		deduper:   o.gatekeeperDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.TenantID,
		source:    "photo_analysis_complete",
	})
}
func (o *Orchestrator) OnPhotoAnalysisFailed(ctx context.Context, evt events.PhotoAnalysisFailed) {
	o.log.Warn("orchestrator: photo analysis failed", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "errorCode", evt.ErrorCode)
	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for photo analysis failure", "error", err)
		return
	}

	if !o.ShouldRunAgent(svc) {
		return
	}

	summary := "Foto-analyse mislukt. Intake loopt door zonder visuele context."
	if evt.ErrorCode == "persistence_failed" {
		summary = "Foto-analyse opgeslagen mislukt. Handmatige controle aanbevolen."
	}

	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         evt.LeadID,
		ServiceID:      &evt.LeadServiceID,
		OrganizationID: evt.TenantID,
		ActorType:      repository.ActorTypeSystem,
		ActorName:      repository.ActorNameOrchestrator,
		EventType:      repository.EventTypeAlert,
		Title:          repository.EventTitlePhotoAnalysisFailed,
		Summary:        &summary,
		Metadata: repository.AlertMetadata{
			Trigger:      "photo_analysis_failed",
			ErrorCode:    evt.ErrorCode,
			ErrorMessage: evt.ErrorMessage,
		}.ToMap(),
	})

	if !domain.AllowsGatekeeperEvaluation(svc.PipelineStage) {
		o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "skip", "reason", "photo_failure_stage_not_eligible", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "stage", svc.PipelineStage)
		return
	}

	if o.automationQueue == nil {
		o.log.Error("orchestrator: automation queue not configured after photo analysis failure", "serviceId", evt.LeadServiceID)
		return
	}
	o.log.Info(orchestratorAutomationLog, "agent", "gatekeeper", "decision", "enqueue", "reason", "photo_analysis_failed", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID, "errorCode", evt.ErrorCode)
	_ = maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{
		ctx:       ctx,
		repo:      o.repo,
		deduper:   o.gatekeeperDeduper,
		queue:     o.automationQueue,
		log:       o.log,
		leadID:    evt.LeadID,
		serviceID: evt.LeadServiceID,
		tenantID:  evt.TenantID,
		source:    "photo_analysis_failed",
	})
}
