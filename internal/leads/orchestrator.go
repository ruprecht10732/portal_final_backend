package leads

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/logger"
)

// Orchestrator routes pipeline events to specialized agents.
type Orchestrator struct {
	gatekeeper *agent.Gatekeeper
	estimator  *agent.Estimator
	dispatcher *agent.Dispatcher
	repo       repository.LeadsRepository
	eventBus   events.Bus
	sse        *sse.Service
	log        *logger.Logger

	reconciliationEnabled bool

	// Idempotency protection: tracks active agent runs
	activeRuns map[string]bool
	// Latest queued photo-analysis event per service, replayed after current gatekeeper run finishes.
	pendingGatekeeperPhoto map[uuid.UUID]events.PhotoAnalysisCompleted
	runsMu                 sync.Mutex
}

const (
	dispatcherAlreadyRunningMsg = "orchestrator: dispatcher already running for service, skipping"
	dispatcherFailedMsg         = "orchestrator: dispatcher failed"
	agentRunTimeout             = 5 * time.Minute
	staleDraftDuration          = 30 * 24 * time.Hour
)

func NewOrchestrator(gatekeeper *agent.Gatekeeper, estimator *agent.Estimator, dispatcher *agent.Dispatcher, repo repository.LeadsRepository, eventBus events.Bus, sse *sse.Service, log *logger.Logger) *Orchestrator {
	return &Orchestrator{
		gatekeeper:             gatekeeper,
		estimator:              estimator,
		dispatcher:             dispatcher,
		repo:                   repo,
		eventBus:               eventBus,
		sse:                    sse,
		log:                    log,
		reconciliationEnabled:  true,
		activeRuns:             make(map[string]bool),
		pendingGatekeeperPhoto: make(map[uuid.UUID]events.PhotoAnalysisCompleted),
	}
}

func (o *Orchestrator) SetReconciliationEnabled(enabled bool) {
	o.reconciliationEnabled = enabled
}

func (o *Orchestrator) queueGatekeeperPhotoRerun(evt events.PhotoAnalysisCompleted) {
	o.runsMu.Lock()
	defer o.runsMu.Unlock()
	if _, exists := o.pendingGatekeeperPhoto[evt.LeadServiceID]; exists {
		o.log.Warn("orchestrator: overwriting queued photo-analysis rerun", "serviceId", evt.LeadServiceID)
	}
	o.pendingGatekeeperPhoto[evt.LeadServiceID] = evt
}

func (o *Orchestrator) popQueuedGatekeeperPhotoRerun(serviceID uuid.UUID) (events.PhotoAnalysisCompleted, bool) {
	o.runsMu.Lock()
	defer o.runsMu.Unlock()
	evt, ok := o.pendingGatekeeperPhoto[serviceID]
	if !ok {
		return events.PhotoAnalysisCompleted{}, false
	}
	delete(o.pendingGatekeeperPhoto, serviceID)
	return evt, true
}

func (o *Orchestrator) canRunGatekeeperForPhotoEvent(ctx context.Context, evt events.PhotoAnalysisCompleted) bool {
	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service for photo analysis event", "error", err)
		return false
	}

	if !o.ShouldRunAgent(svc) {
		return false
	}

	if svc.PipelineStage != domain.PipelineStageTriage && svc.PipelineStage != domain.PipelineStageNurturing {
		return false
	}

	return true
}

func (o *Orchestrator) runGatekeeperForPhotoEvent(evt events.PhotoAnalysisCompleted) {
	o.log.Info("orchestrator: photo analysis complete, waking gatekeeper", "leadId", evt.LeadID, "summary", evt.Summary)
	go func(current events.PhotoAnalysisCompleted) {
		defer o.markComplete("gatekeeper", current.LeadServiceID)

		for {
			runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
			err := o.gatekeeper.Run(runCtx, current.LeadID, current.LeadServiceID, current.TenantID)
			cancel()
			if err != nil {
				o.log.Error("orchestrator: gatekeeper failed", "error", err)
			}

			next, ok := o.popQueuedGatekeeperPhotoRerun(current.LeadServiceID)
			if !ok {
				return
			}

			if !o.canRunGatekeeperForPhotoEvent(context.Background(), next) {
				return
			}

			o.log.Info("orchestrator: replaying queued gatekeeper rerun after photo analysis", "leadId", next.LeadID, "serviceId", next.LeadServiceID)
			current = next
		}
	}(evt)
}

// markRunning attempts to mark an agent run as active. Returns true if successfully marked, false if already running.
func (o *Orchestrator) markRunning(agentName string, serviceID uuid.UUID) bool {
	o.runsMu.Lock()
	defer o.runsMu.Unlock()

	key := agentName + ":" + serviceID.String()
	if o.activeRuns[key] {
		return false // Already running
	}
	o.activeRuns[key] = true
	return true
}

// markComplete removes the active run marker.
func (o *Orchestrator) markComplete(agentName string, serviceID uuid.UUID) {
	o.runsMu.Lock()
	defer o.runsMu.Unlock()

	key := agentName + ":" + serviceID.String()
	delete(o.activeRuns, key)
}

func (o *Orchestrator) recordDispatcherFailure(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) {
	summary := "Partner matching mislukt. Probeer opnieuw."
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      "System",
		ActorName:      "Dispatcher",
		EventType:      "alert",
		Title:          "Partner matching mislukt",
		Summary:        &summary,
		Metadata: map[string]any{
			"trigger": "dispatcher_run",
		},
	})
}

// ShouldRunAgent checks if a service is eligible for agent processing.
// Returns false if the service is in a terminal state.
func (o *Orchestrator) ShouldRunAgent(service repository.LeadService) bool {
	if domain.IsTerminal(service.Status, service.PipelineStage) {
		o.log.Info("orchestrator: skipping agent run for terminal service",
			"serviceId", service.ID,
			"status", service.Status,
			"pipelineStage", service.PipelineStage)
		return false
	}
	return true
}

// OnDataChange handles human data changes and re-triggers agents when needed.
func (o *Orchestrator) OnDataChange(ctx context.Context, evt events.LeadDataChanged) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.TenantID, evt.EventName(), evt.OccurredAt(), false)

	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service", "error", err)
		return
	}

	if !o.ShouldRunAgent(svc) {
		return
	}

	if svc.PipelineStage == "Triage" || svc.PipelineStage == "Nurturing" || svc.PipelineStage == "Manual_Intervention" {
		// Idempotency check
		if !o.markRunning("gatekeeper", evt.LeadServiceID) {
			o.log.Info("orchestrator: gatekeeper already running for service, skipping", "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: data changed, waking gatekeeper", "leadId", evt.LeadID, "stage", svc.PipelineStage)
		go func() {
			defer o.markComplete("gatekeeper", evt.LeadServiceID)
			runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
			defer cancel()
			if err := o.gatekeeper.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: gatekeeper failed", "error", err)
			}
		}()
		return
	}
}

// OnPhotoAnalysisCompleted triggers gatekeeper re-evaluation once visual data is available.
func (o *Orchestrator) OnPhotoAnalysisCompleted(ctx context.Context, evt events.PhotoAnalysisCompleted) {
	if !o.canRunGatekeeperForPhotoEvent(ctx, evt) {
		return
	}

	if !o.markRunning("gatekeeper", evt.LeadServiceID) {
		o.queueGatekeeperPhotoRerun(evt)
		o.log.Info("orchestrator: gatekeeper already running, queued photo-analysis rerun", "serviceId", evt.LeadServiceID)
		return
	}

	o.runGatekeeperForPhotoEvent(evt)
}

// OnPhotoAnalysisFailed records failure context and wakes gatekeeper explicitly when useful.
func (o *Orchestrator) OnPhotoAnalysisFailed(ctx context.Context, evt events.PhotoAnalysisFailed) {
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
		ActorType:      "System",
		ActorName:      "Orchestrator",
		EventType:      "alert",
		Title:          "Foto-analyse mislukt",
		Summary:        &summary,
		Metadata: map[string]any{
			"trigger":      "photo_analysis_failed",
			"errorCode":    evt.ErrorCode,
			"errorMessage": evt.ErrorMessage,
		},
	})

	if svc.PipelineStage != domain.PipelineStageTriage && svc.PipelineStage != domain.PipelineStageNurturing {
		return
	}

	if !o.markRunning("gatekeeper", evt.LeadServiceID) {
		o.log.Info("orchestrator: gatekeeper already running for failed photo analysis", "serviceId", evt.LeadServiceID)
		return
	}

	o.log.Info("orchestrator: photo analysis failed, waking gatekeeper", "leadId", evt.LeadID, "errorCode", evt.ErrorCode)
	go func() {
		defer o.markComplete("gatekeeper", evt.LeadServiceID)
		runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
		defer cancel()
		if err := o.gatekeeper.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
			o.log.Error("orchestrator: gatekeeper failed after photo analysis failure", "error", err)
		}
	}()
}

// OnStageChange triggers downstream agents based on pipeline transitions.
func (o *Orchestrator) OnStageChange(ctx context.Context, evt events.PipelineStageChanged) {
	// Terminal stages never trigger agents
	if domain.IsTerminalPipelineStage(evt.NewStage) {
		return
	}
	// Intentionally no generic stage-change timeline write here.
	// Agent tools already persist detailed stage-change reasons.

	switch evt.NewStage {
	case domain.PipelineStageReadyForEstimator:
		// Idempotency check
		if !o.markRunning("estimator", evt.LeadServiceID) {
			o.log.Info("orchestrator: estimator already running for service, skipping", "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: lead ready for estimation", "leadId", evt.LeadID)
		go func() {
			defer o.markComplete("estimator", evt.LeadServiceID)
			runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
			defer cancel()
			if err := o.estimator.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: estimator failed", "error", err)
			}
		}()

	case domain.PipelineStageReadyForPartner:
		// Idempotency check
		if !o.markRunning("dispatcher", evt.LeadServiceID) {
			o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: lead ready for dispatch", "leadId", evt.LeadID)
		go func() {
			defer o.markComplete("dispatcher", evt.LeadServiceID)
			runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
			defer cancel()
			if err := o.dispatcher.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error(dispatcherFailedMsg, "error", err)
				o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID)
			}
		}()

	case domain.PipelineStageManualIntervention:
		o.log.Warn("orchestrator: manual intervention required", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
		// Record timeline event for audit trail
		drafts := buildManualInterventionDrafts(evt.LeadID, evt.LeadServiceID)
		_, _ = o.repo.CreateTimelineEvent(context.Background(), repository.CreateTimelineEventParams{
			LeadID:         evt.LeadID,
			ServiceID:      &evt.LeadServiceID,
			OrganizationID: evt.TenantID,
			ActorType:      "System",
			ActorName:      "Orchestrator",
			EventType:      "alert",
			Title:          "Handmatige interventie vereist",
			Summary:        stringPtr("Geautomatiseerde verwerking vereist menselijke beoordeling"),
			Metadata: map[string]any{
				"previous_stage": evt.OldStage,
				"trigger":        "pipeline_stage_change",
				"drafts":         drafts,
			},
		})

		// Push real-time notification to all org members via SSE
		o.sse.PublishToOrganization(evt.TenantID, sse.Event{
			Type:      sse.EventManualIntervention,
			LeadID:    evt.LeadID,
			ServiceID: evt.LeadServiceID,
			Message:   "Geautomatiseerde verwerking vereist menselijke beoordeling",
			Data: map[string]any{
				"previousStage": evt.OldStage,
			},
		})

		// Publish domain event for downstream handlers (email notifications, etc.)
		o.eventBus.Publish(context.Background(), events.ManualInterventionRequired{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        evt.LeadID,
			LeadServiceID: evt.LeadServiceID,
			TenantID:      evt.TenantID,
			Reason:        "pipeline_stage_change",
			Context:       "Transitioned from " + evt.OldStage,
		})
	}
}

// OnQuoteAccepted advances pipeline after customer approval.
func (o *Orchestrator) OnQuoteAccepted(ctx context.Context, evt events.QuoteAccepted) {
	if evt.LeadServiceID == nil {
		return
	}
	serviceID := *evt.LeadServiceID

	oldStage := ""
	if svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, evt.OrganizationID); err == nil {
		oldStage = svc.PipelineStage
	}

	summary := fmt.Sprintf("Offerte %s geaccepteerd. Starten met zoeken naar partner.", evt.QuoteNumber)
	_, _ = o.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         evt.LeadID,
		ServiceID:      evt.LeadServiceID,
		OrganizationID: evt.OrganizationID,
		ActorType:      "System",
		ActorName:      "Orchestrator",
		EventType:      "stage_change",
		Title:          "Offerte Geaccepteerd",
		Summary:        &summary,
		Metadata:       map[string]any{"quoteId": evt.QuoteID},
	})

	if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, evt.OrganizationID, domain.PipelineStageReadyForPartner); err != nil {
		o.log.Error("orchestrator: failed to advance stage after quote acceptance", "error", err)
		return
	}

	if _, err := o.repo.UpdateServiceStatus(ctx, serviceID, evt.OrganizationID, domain.LeadStatusQuoteAccepted); err != nil {
		o.log.Error("orchestrator: failed to set service status after quote acceptance", "error", err)
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      domain.PipelineStageReadyForPartner,
	})
}

func (o *Orchestrator) OnQuoteRejected(ctx context.Context, evt events.QuoteRejected) {
	if evt.LeadServiceID == nil {
		return
	}

	serviceID := *evt.LeadServiceID
	oldStage := ""
	if svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, evt.OrganizationID); err == nil {
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
		ActorType:      "System",
		ActorName:      "Orchestrator",
		EventType:      "stage_change",
		Title:          "Offerte Afgewezen",
		Summary:        &summary,
		Metadata:       map[string]any{"quoteId": evt.QuoteID, "reason": evt.Reason},
	})

	if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, evt.OrganizationID, domain.PipelineStageLost); err != nil {
		o.log.Error("orchestrator: failed to advance stage after quote rejection", "error", err)
		return
	}

	if _, err := o.repo.UpdateServiceStatus(ctx, serviceID, evt.OrganizationID, domain.LeadStatusLost); err != nil {
		o.log.Error("orchestrator: failed to set service status after quote rejection", "error", err)
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

	if svc.PipelineStage == domain.PipelineStageQuoteSent {
		return
	}

	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		return
	}

	oldStage := svc.PipelineStage
	if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, evt.OrganizationID, domain.PipelineStageQuoteSent); err != nil {
		o.log.Error("orchestrator: failed to advance stage after quote sent", "error", err)
		return
	}

	if _, err := o.repo.UpdateServiceStatus(ctx, serviceID, evt.OrganizationID, domain.LeadStatusQuoteSent); err != nil {
		o.log.Error("orchestrator: failed to set service status after quote sent", "error", err)
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      domain.PipelineStageQuoteSent,
	})
}

// OnPartnerOfferRejected re-triggers the dispatcher to find a new partner.
func (o *Orchestrator) OnPartnerOfferRejected(ctx context.Context, evt events.PartnerOfferRejected) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
	o.log.Info("Orchestrator: Partner rejected offer, re-triggering dispatcher", "leadId", evt.LeadID)
	if !o.markRunning("dispatcher", evt.LeadServiceID) {
		o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
		return
	}
	go func() {
		defer o.markComplete("dispatcher", evt.LeadServiceID)
		runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
		defer cancel()
		if err := o.dispatcher.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID); err != nil {
			o.log.Error(dispatcherFailedMsg, "error", err)
			o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID)
		}
	}()
}

// OnPartnerOfferExpired re-triggers the dispatcher when an offer expires.
func (o *Orchestrator) OnPartnerOfferExpired(ctx context.Context, evt events.PartnerOfferExpired) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
	o.log.Info("Orchestrator: Partner offer expired, re-triggering dispatcher", "leadId", evt.LeadID)
	if !o.markRunning("dispatcher", evt.LeadServiceID) {
		o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
		return
	}
	go func() {
		defer o.markComplete("dispatcher", evt.LeadServiceID)
		runCtx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
		defer cancel()
		if err := o.dispatcher.Run(runCtx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID); err != nil {
			o.log.Error(dispatcherFailedMsg, "error", err)
			o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID)
		}
	}()
}

// OnPartnerOfferAccepted advances pipeline once a partner accepts the offer.
func (o *Orchestrator) OnPartnerOfferAccepted(ctx context.Context, evt events.PartnerOfferAccepted) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}

func (o *Orchestrator) OnPartnerOfferCreated(ctx context.Context, evt events.PartnerOfferCreated) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}

func stringPtr(s string) *string {
	return &s
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

// -------------------------------------------------------------------------
// Context-aware Service State Reconciliation
// -------------------------------------------------------------------------

func (o *Orchestrator) OnQuoteCreated(ctx context.Context, evt events.QuoteCreated) {
	if evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}

func (o *Orchestrator) OnQuoteDeleted(ctx context.Context, evt events.QuoteDeleted) {
	if evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
}

func (o *Orchestrator) OnAppointmentCreated(ctx context.Context, evt events.AppointmentCreated) {
	if evt.LeadID == nil || evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, *evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}

func (o *Orchestrator) OnAppointmentStatusChanged(ctx context.Context, evt events.AppointmentStatusChanged) {
	if evt.LeadID == nil || evt.LeadServiceID == nil {
		return
	}

	allowResurrection := evt.NewStatus == "scheduled" || evt.NewStatus == "requested"
	o.reconcileServiceState(ctx, *evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), allowResurrection)
}

// reconcileServiceState is the single source of truth for LeadService state.
// It derives pipeline stage + service status from child entities and enforces consistency.
func (o *Orchestrator) reconcileServiceState(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, triggerEvent string, triggerAt time.Time, allowResurrection bool) {
	if !o.reconciliationEnabled {
		return
	}

	svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load service for reconciliation", "error", err)
		return
	}

	aggs, err := o.repo.GetServiceStateAggregates(ctx, serviceID, tenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load aggregates", "error", err)
		return
	}

	desired, ok := deriveDesiredServiceState(svc, aggs, allowResurrection, triggerAt)
	if !ok {
		return
	}

	o.applyReconciledState(ctx, applyReconciledStateParams{
		LeadID:       leadID,
		ServiceID:    serviceID,
		TenantID:     tenantID,
		TriggerEvent: triggerEvent,
		Current:      svc,
		Desired:      desired,
		Aggregates:   aggs,
	})
}

type applyReconciledStateParams struct {
	LeadID       uuid.UUID
	ServiceID    uuid.UUID
	TenantID     uuid.UUID
	TriggerEvent string
	Current      repository.LeadService
	Desired      desiredServiceState
	Aggregates   repository.ServiceStateAggregates
}

func (o *Orchestrator) applyReconciledState(ctx context.Context, p applyReconciledStateParams) {
	oldStage := p.Current.PipelineStage
	oldStatus := p.Current.Status

	stageChanged := o.applyReconciledStage(ctx, p.LeadID, p.ServiceID, p.TenantID, oldStage, p.Desired.Stage)
	statusChanged := o.applyReconciledStatus(ctx, p.ServiceID, p.TenantID, oldStatus, p.Desired.Status)
	if !stageChanged && !statusChanged {
		return
	}

	reason := p.Desired.Reason
	if reason == "" {
		reason = defaultReconcileReason(p.TriggerEvent, oldStage, p.Desired.Stage)
	}

	o.maybeWriteReconcileTimeline(ctx, maybeWriteTimelineParams{
		LeadID:       p.LeadID,
		ServiceID:    p.ServiceID,
		TenantID:     p.TenantID,
		TriggerEvent: p.TriggerEvent,
		OldStage:     oldStage,
		NewStage:     p.Desired.Stage,
		OldStatus:    oldStatus,
		NewStatus:    p.Desired.Status,
		ReasonCode:   p.Desired.ReasonCode,
		Reason:       reason,
		Resurrecting: p.Desired.Resurrecting,
		Aggregates:   p.Aggregates,
	})

	o.log.Info("orchestrator: reconciled service state",
		"leadId", p.LeadID,
		"serviceId", p.ServiceID,
		"trigger", p.TriggerEvent,
		"reason", reason,
		"oldStage", oldStage,
		"newStage", p.Desired.Stage,
		"oldStatus", oldStatus,
		"newStatus", p.Desired.Status,
	)
}

func (o *Orchestrator) applyReconciledStage(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, oldStage, newStage string) bool {
	if newStage == "" || newStage == oldStage {
		return false
	}
	if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, tenantID, newStage); err != nil {
		o.log.Error("orchestrator: failed to reconcile stage", "error", err)
		return false
	}
	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		TenantID:      tenantID,
		OldStage:      oldStage,
		NewStage:      newStage,
	})
	return true
}

func (o *Orchestrator) applyReconciledStatus(ctx context.Context, serviceID, tenantID uuid.UUID, oldStatus, newStatus string) bool {
	if newStatus == "" || newStatus == oldStatus {
		return false
	}
	if _, err := o.repo.UpdateServiceStatus(ctx, serviceID, tenantID, newStatus); err != nil {
		o.log.Error("orchestrator: failed to reconcile status", "error", err)
		return false
	}
	return true
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
		ActorType:      "System",
		ActorName:      "StateReconciler",
		EventType:      "service_state_reconciled",
		Title:          "Status automatisch gecorrigeerd",
		Summary:        &summary,
		Metadata: map[string]any{
			"reasonCode": p.ReasonCode,
			"trigger":    p.TriggerEvent,
			"oldStage":   p.OldStage,
			"newStage":   p.NewStage,
			"oldStatus":  p.OldStatus,
			"newStatus":  p.NewStatus,
			"evidence":   buildReconcileEvidence(p.Aggregates),
		},
	})
}

type desiredServiceState struct {
	Stage        string
	Status       string
	ReasonCode   string
	Reason       string
	Resurrecting bool
}

func deriveDesiredServiceState(current repository.LeadService, aggs repository.ServiceStateAggregates, allowResurrection bool, triggerAt time.Time) (desiredServiceState, bool) {
	resurrecting := shouldResurrect(current, aggs, allowResurrection, triggerAt)
	if domain.IsTerminal(current.Status, current.PipelineStage) && !resurrecting {
		return desiredServiceState{}, false
	}

	desired := desiredServiceState{Resurrecting: resurrecting}
	if resurrecting {
		desired.ReasonCode = "terminal_resurrection"
		desired.Reason = "Lead automatisch heropend door nieuwe activiteit"
	}

	if stage, status, code, ok := deriveFromOffers(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, reason, ok := deriveFromQuotes(aggs); ok {
		desired.Stage, desired.Status = stage, status
		desired.ReasonCode = coalesceReasonCode(desired.ReasonCode, code)
		if desired.Reason == "" {
			desired.Reason = reason
		}
		return desired, true
	}
	if stage, status, code, ok := deriveFromVisitReports(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, ok := deriveFromAppointments(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, ok := deriveFromAI(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}

	desired.Stage = domain.PipelineStageTriage
	desired.Status = domain.LeadStatusNew
	desired.ReasonCode = coalesceReasonCode(desired.ReasonCode, "default")
	return desired, true
}

func shouldResurrect(current repository.LeadService, aggs repository.ServiceStateAggregates, allowResurrection bool, triggerAt time.Time) bool {
	if !domain.IsTerminal(current.Status, current.PipelineStage) {
		return false
	}
	if !allowResurrection {
		return false
	}

	// Time-safety: only resurrect if this triggering event happened AFTER the service became terminal.
	// This prevents replays/duplicates of old events from reopening long-closed services.
	terminalAt := aggs.TerminalAt
	if terminalAt == nil {
		// Fallback: the service row's updated_at is updated on pipeline/status changes.
		// Not perfect, but safer than allowing resurrection without a time barrier.
		fallback := current.UpdatedAt
		terminalAt = &fallback
	}
	if terminalAt != nil && !triggerAt.After(*terminalAt) {
		return false
	}

	return aggs.ScheduledAppointments > 0 || aggs.AcceptedOffers > 0 || aggs.PendingOffers > 0 || aggs.AcceptedQuotes > 0 || aggs.SentQuotes > 0 || aggs.DraftQuotes > 0
}

func deriveFromOffers(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.AcceptedOffers > 0 {
		return domain.PipelineStagePartnerAssigned, domain.LeadStatusPartnerAssigned, "offer_accepted", true
	}
	if aggs.PendingOffers > 0 {
		return domain.PipelineStagePartnerMatching, domain.LeadStatusQuoteAccepted, "offer_pending", true
	}
	return "", "", "", false
}

func deriveFromQuotes(aggs repository.ServiceStateAggregates) (stage, status, reasonCode, reason string, ok bool) {
	if aggs.AcceptedQuotes > 0 {
		return domain.PipelineStageReadyForPartner, domain.LeadStatusQuoteAccepted, "quote_accepted", "", true
	}
	if aggs.SentQuotes > 0 {
		return domain.PipelineStageQuoteSent, domain.LeadStatusQuoteSent, "quote_sent", "", true
	}
	if aggs.DraftQuotes > 0 {
		if aggs.LatestQuoteAt != nil && time.Since(*aggs.LatestQuoteAt) > staleDraftDuration {
			return domain.PipelineStageNurturing, domain.LeadStatusAttemptedContact, "stale_draft_decay", "Conceptofferte is verlopen (>30 dagen geen activiteit)", true
		}
		return domain.PipelineStageQuoteDraft, domain.LeadStatusQuoteDraft, "quote_draft", "", true
	}
	if aggs.RejectedQuotes > 0 {
		return domain.PipelineStageLost, domain.LeadStatusLost, "quotes_rejected_only", "", true
	}
	return "", "", "", "", false
}

func deriveFromVisitReports(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if !aggs.HasVisitReport {
		return "", "", "", false
	}
	return domain.PipelineStageReadyForEstimator, domain.LeadStatusSurveyCompleted, "visit_report_present", true
}

func deriveFromAppointments(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.ScheduledAppointments > 0 {
		return domain.PipelineStageNurturing, domain.LeadStatusAppointmentScheduled, "appointment_scheduled", true
	}
	if aggs.CancelledAppointments > 0 {
		return domain.PipelineStageNurturing, domain.LeadStatusNeedsRescheduling, "appointment_cancelled", true
	}
	return "", "", "", false
}

func deriveFromAI(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.AiAction == nil {
		return "", "", "", false
	}
	switch *aggs.AiAction {
	case "ScheduleSurvey", "CallImmediately", "Review":
		return domain.PipelineStageReadyForEstimator, domain.LeadStatusNew, "ai_valid_intake", true
	case "RequestInfo":
		return domain.PipelineStageNurturing, domain.LeadStatusAttemptedContact, "ai_request_info", true
	case "Reject":
		return domain.PipelineStageLost, domain.LeadStatusDisqualified, "ai_reject", true
	default:
		return domain.PipelineStageTriage, domain.LeadStatusNew, "ai_default", true
	}
}

func defaultReconcileReason(triggerEvent, oldStage, newStage string) string {
	switch triggerEvent {
	case events.QuoteDeleted{}.EventName():
		return "Offerte verwijderd; status opnieuw bepaald"
	case events.AppointmentStatusChanged{}.EventName():
		return "Afspraakstatus gewijzigd; status opnieuw bepaald"
	case events.AppointmentCreated{}.EventName():
		return "Nieuwe afspraak; status opnieuw bepaald"
	default:
		return fmt.Sprintf("Auto-correctie: %s → %s", oldStage, newStage)
	}
}

func buildReconcileEvidence(aggs repository.ServiceStateAggregates) map[string]any {
	return map[string]any{
		"acceptedQuotes":        aggs.AcceptedQuotes,
		"sentQuotes":            aggs.SentQuotes,
		"draftQuotes":           aggs.DraftQuotes,
		"rejectedQuotes":        aggs.RejectedQuotes,
		"latestQuoteAt":         aggs.LatestQuoteAt,
		"acceptedOffers":        aggs.AcceptedOffers,
		"pendingOffers":         aggs.PendingOffers,
		"scheduledAppointments": aggs.ScheduledAppointments,
		"completedAppointments": aggs.CompletedAppointments,
		"cancelledAppointments": aggs.CancelledAppointments,
		"latestAppointmentAt":   aggs.LatestAppointmentAt,
		"hasVisitReport":        aggs.HasVisitReport,
		"aiAction":              aggs.AiAction,
	}
}

func coalesceReasonCode(existing, next string) string {
	if existing != "" {
		return existing
	}
	return next
}

func isPipelineRegression(oldStage, newStage string) bool {
	rank := map[string]int{
		domain.PipelineStageTriage:             10,
		domain.PipelineStageNurturing:          20,
		domain.PipelineStageManualIntervention: 25,
		domain.PipelineStageReadyForEstimator:  30,
		domain.PipelineStageQuoteDraft:         40,
		domain.PipelineStageQuoteSent:          50,
		domain.PipelineStageReadyForPartner:    60,
		domain.PipelineStagePartnerMatching:    70,
		domain.PipelineStagePartnerAssigned:    80,
		domain.PipelineStageCompleted:          90,
		domain.PipelineStageLost:               90,
	}

	oldRank, okOld := rank[oldStage]
	newRank, okNew := rank[newStage]
	if !okOld || !okNew {
		return false
	}
	return newRank < oldRank
}
