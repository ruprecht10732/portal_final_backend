package leads

import (
	"context"

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
}

func NewOrchestrator(gatekeeper *agent.Gatekeeper, estimator *agent.Estimator, dispatcher *agent.Dispatcher, repo repository.LeadsRepository, log *logger.Logger) *Orchestrator {
	return &Orchestrator{
		gatekeeper: gatekeeper,
		estimator:  estimator,
		dispatcher: dispatcher,
		repo:       repo,
		log:        log,
	}
}

// OnDataChange handles human data changes and re-triggers agents when needed.
func (o *Orchestrator) OnDataChange(ctx context.Context, evt events.LeadDataChanged) {
	svc, err := o.repo.GetLeadServiceByID(ctx, evt.LeadServiceID, evt.TenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load lead service", "error", err)
		return
	}

	if svc.PipelineStage == "Triage" || svc.PipelineStage == "Nurturing" {
		o.log.Info("orchestrator: data changed, waking gatekeeper", "leadId", evt.LeadID)
		go func() {
			if err := o.gatekeeper.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: gatekeeper failed", "error", err)
			}
		}()
		return
	}

	if svc.PipelineStage == "Manual_Intervention" {
		o.log.Info("orchestrator: data changed in manual mode, retrying estimation", "leadId", evt.LeadID)
		go func() {
			if err := o.estimator.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: estimator failed", "error", err)
			}
		}()
	}
}

// OnStageChange triggers downstream agents based on pipeline transitions.
func (o *Orchestrator) OnStageChange(ctx context.Context, evt events.PipelineStageChanged) {
	switch evt.NewStage {
	case "Ready_For_Estimator":
		o.log.Info("orchestrator: lead ready for estimation", "leadId", evt.LeadID)
		go func() {
			if err := o.estimator.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: estimator failed", "error", err)
			}
		}()

	case "Ready_For_Partner":
		o.log.Info("orchestrator: lead ready for dispatch", "leadId", evt.LeadID)
		go func() {
			if err := o.dispatcher.Run(context.Background(), evt.LeadID, evt.LeadServiceID, evt.TenantID); err != nil {
				o.log.Error("orchestrator: dispatcher failed", "error", err)
			}
		}()
	}
}
