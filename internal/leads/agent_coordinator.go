package leads

import (
	"context"

	"portal_final_backend/internal/events"
)

// AgentCoordinator routes agent-oriented triggers (data and photo events).
type AgentCoordinator struct {
	orchestrator *Orchestrator
}

func newAgentCoordinator(orchestrator *Orchestrator) *AgentCoordinator {
	return &AgentCoordinator{orchestrator: orchestrator}
}

func (c *AgentCoordinator) OnLeadDataChanged(ctx context.Context, evt events.LeadDataChanged) {
	c.orchestrator.OnDataChange(ctx, evt)
}

func (c *AgentCoordinator) OnPhotoAnalysisCompleted(ctx context.Context, evt events.PhotoAnalysisCompleted) {
	c.orchestrator.OnPhotoAnalysisCompleted(ctx, evt)
}

func (c *AgentCoordinator) OnPhotoAnalysisFailed(ctx context.Context, evt events.PhotoAnalysisFailed) {
	c.orchestrator.OnPhotoAnalysisFailed(ctx, evt)
}

func (c *AgentCoordinator) OnVisitReportSubmitted(ctx context.Context, evt events.VisitReportSubmitted) {
	c.orchestrator.OnVisitReportSubmitted(ctx, evt)
}

func subscribeAgentCoordinator(eventBus events.Bus, coordinator *AgentCoordinator) {
	eventBus.Subscribe(events.LeadDataChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadDataChanged)
		if !ok {
			return nil
		}
		coordinator.OnLeadDataChanged(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PhotoAnalysisCompleted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PhotoAnalysisCompleted)
		if !ok {
			return nil
		}
		coordinator.OnPhotoAnalysisCompleted(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PhotoAnalysisFailed{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PhotoAnalysisFailed)
		if !ok {
			return nil
		}
		coordinator.OnPhotoAnalysisFailed(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.VisitReportSubmitted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.VisitReportSubmitted)
		if !ok {
			return nil
		}
		coordinator.OnVisitReportSubmitted(ctx, e)
		return nil
	}))
}
