package leads

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"
)

// Orchestrator routes pipeline events to specialized agents.
type Orchestrator struct {
	gatekeeper *agent.Gatekeeper
	estimator  *agent.Estimator
	dispatcher *agent.Dispatcher
	repo       repository.LeadsRepository
	log        *logger.Logger

	// Idempotency protection: tracks active agent runs
	activeRuns map[string]bool
	runsMu     sync.Mutex
}

func NewOrchestrator(gatekeeper *agent.Gatekeeper, estimator *agent.Estimator, dispatcher *agent.Dispatcher, repo repository.LeadsRepository, log *logger.Logger) *Orchestrator {
	return &Orchestrator{
		gatekeeper: gatekeeper,
		estimator:  estimator,
		dispatcher: dispatcher,
		repo:       repo,
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

// OnDataChange handles human data changes and re-triggers agents when needed.
func (o *Orchestrator) OnDataChange(ctx context.Context, evt events.LeadDataChanged) {
	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service", "error", err)
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
			o.log.Info("orchestrator: dispatcher already running for service, skipping", "serviceId", evt.LeadServiceID)
			return
		}

		o.log.Info("orchestrator: lead ready for dispatch", "leadId", evt.LeadID)
		go func() {
			defer o.markComplete("dispatcher", evt.LeadServiceID)
			if err := o.dispatcher.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: dispatcher failed", "error", err)
			}
		}()

	case "Manual_Intervention":
		o.log.Warn("orchestrator: manual intervention required", "leadId", evt.LeadID, "serviceId", evt.LeadServiceID)
		// Publish ManualInterventionRequired event for admin notifications
		o.repo.CreateTimelineEvent(context.Background(), repository.CreateTimelineEventParams{
			LeadID:         evt.LeadID,
			ServiceID:      &evt.LeadServiceID,
			OrganizationID: evt.TenantID,
			ActorType:      "System",
			ActorName:      "Orchestrator",
			EventType:      "alert",
			Title:          "Manual intervention required",
			Summary:        stringPtr("Automated processing requires human review"),
			Metadata: map[string]any{
				"previous_stage": evt.OldStage,
				"trigger":        "pipeline_stage_change",
			},
		})
		// TODO: Publish ManualInterventionRequired event to SSE/notification system when ready
	}
}

func stringPtr(s string) *string {
	return &s
}
