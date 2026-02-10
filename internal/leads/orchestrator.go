package leads

import (
	"context"
	"fmt"
	"sync"

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

	// Idempotency protection: tracks active agent runs
	activeRuns map[string]bool
	runsMu     sync.Mutex
}

const (
	dispatcherAlreadyRunningMsg = "orchestrator: dispatcher already running for service, skipping"
	dispatcherFailedMsg         = "orchestrator: dispatcher failed"
)

func NewOrchestrator(gatekeeper *agent.Gatekeeper, estimator *agent.Estimator, dispatcher *agent.Dispatcher, repo repository.LeadsRepository, eventBus events.Bus, sse *sse.Service, log *logger.Logger) *Orchestrator {
	return &Orchestrator{
		gatekeeper: gatekeeper,
		estimator:  estimator,
		dispatcher: dispatcher,
		repo:       repo,
		eventBus:   eventBus,
		sse:        sse,
		log:        log,
		activeRuns: make(map[string]bool),
	}
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
// Returns false if the service is in a terminal state (Closed, Bad_Lead, Surveyed,
// Completed, or Lost).
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
			if err := o.gatekeeper.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: gatekeeper failed", "error", err)
			}
		}()
		return
	}
}

// OnStageChange triggers downstream agents based on pipeline transitions.
func (o *Orchestrator) OnStageChange(ctx context.Context, evt events.PipelineStageChanged) {
	// Terminal stages never trigger agents
	if domain.IsTerminalPipelineStage(evt.NewStage) {
		return
	}

	switch evt.NewStage {
	case "Ready_For_Estimator":
		// Idempotency check
		if !o.markRunning("estimator", evt.LeadServiceID) {
			o.log.Info("orchestrator: estimator already running for service, skipping", "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: lead ready for estimation", "leadId", evt.LeadID)
		go func() {
			defer o.markComplete("estimator", evt.LeadServiceID)
			if err := o.estimator.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: estimator failed", "error", err)
			}
		}()

	case "Ready_For_Partner":
		// Idempotency check
		if !o.markRunning("dispatcher", evt.LeadServiceID) {
			o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: lead ready for dispatch", "leadId", evt.LeadID)
		go func() {
			defer o.markComplete("dispatcher", evt.LeadServiceID)
			if err := o.dispatcher.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error(dispatcherFailedMsg, "error", err)
				o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID)
			}
		}()

	case "Manual_Intervention":
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

	if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, evt.OrganizationID, "Ready_For_Partner"); err != nil {
		o.log.Error("orchestrator: failed to advance stage after quote acceptance", "error", err)
		return
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: serviceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      "Ready_For_Partner",
	})
}

// OnPartnerOfferRejected re-triggers the dispatcher to find a new partner.
func (o *Orchestrator) OnPartnerOfferRejected(ctx context.Context, evt events.PartnerOfferRejected) {
	_ = ctx
	o.log.Info("Orchestrator: Partner rejected offer, re-triggering dispatcher", "leadId", evt.LeadID)
	if !o.markRunning("dispatcher", evt.LeadServiceID) {
		o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
		return
	}
	go func() {
		defer o.markComplete("dispatcher", evt.LeadServiceID)
		if err := o.dispatcher.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID); err != nil {
			o.log.Error(dispatcherFailedMsg, "error", err)
			o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID)
		}
	}()
}

// OnPartnerOfferExpired re-triggers the dispatcher when an offer expires.
func (o *Orchestrator) OnPartnerOfferExpired(ctx context.Context, evt events.PartnerOfferExpired) {
	_ = ctx
	o.log.Info("Orchestrator: Partner offer expired, re-triggering dispatcher", "leadId", evt.LeadID)
	if !o.markRunning("dispatcher", evt.LeadServiceID) {
		o.log.Info(dispatcherAlreadyRunningMsg, "serviceId", evt.LeadServiceID)
		return
	}
	go func() {
		defer o.markComplete("dispatcher", evt.LeadServiceID)
		if err := o.dispatcher.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID); err != nil {
			o.log.Error(dispatcherFailedMsg, "error", err)
			o.recordDispatcherFailure(context.Background(), evt.LeadID, evt.LeadServiceID, evt.OrganizationID)
		}
	}()
}

// OnPartnerOfferAccepted advances pipeline once a partner accepts the offer.
func (o *Orchestrator) OnPartnerOfferAccepted(ctx context.Context, evt events.PartnerOfferAccepted) {
	oldStage := ""
	if svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.OrganizationID); err == nil {
		oldStage = svc.PipelineStage
	}

	if _, err := o.repo.UpdatePipelineStage(ctx, evt.LeadServiceID, evt.OrganizationID, "Partner_Assigned"); err != nil {
		o.log.Error("orchestrator: failed to advance stage after partner acceptance", "error", err)
		return
	}

	o.eventBus.Publish(ctx, events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        evt.LeadID,
		LeadServiceID: evt.LeadServiceID,
		TenantID:      evt.OrganizationID,
		OldStage:      oldStage,
		NewStage:      "Partner_Assigned",
	})
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
