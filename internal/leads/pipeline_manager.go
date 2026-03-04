package leads

import (
	"context"

	"portal_final_backend/internal/events"
)

// PipelineManager handles stage transitions and business process routing.
type PipelineManager struct {
	orchestrator *Orchestrator
}

func newPipelineManager(orchestrator *Orchestrator) *PipelineManager {
	return &PipelineManager{orchestrator: orchestrator}
}

func (m *PipelineManager) OnLeadAutoDisqualified(ctx context.Context, evt events.LeadAutoDisqualified) {
	m.orchestrator.OnLeadAutoDisqualified(ctx, evt)
}

func (m *PipelineManager) OnQuoteAccepted(ctx context.Context, evt events.QuoteAccepted) {
	m.orchestrator.OnQuoteAccepted(ctx, evt)
}

func (m *PipelineManager) OnQuoteRejected(ctx context.Context, evt events.QuoteRejected) {
	m.orchestrator.OnQuoteRejected(ctx, evt)
}

func (m *PipelineManager) OnQuoteSent(ctx context.Context, evt events.QuoteSent) {
	m.orchestrator.OnQuoteSent(ctx, evt)
}

func (m *PipelineManager) OnPartnerOfferCreated(ctx context.Context, evt events.PartnerOfferCreated) {
	m.orchestrator.OnPartnerOfferCreated(ctx, evt)
}

func (m *PipelineManager) OnPartnerOfferRejected(ctx context.Context, evt events.PartnerOfferRejected) {
	m.orchestrator.OnPartnerOfferRejected(ctx, evt)
}

func (m *PipelineManager) OnPartnerOfferAccepted(ctx context.Context, evt events.PartnerOfferAccepted) {
	m.orchestrator.OnPartnerOfferAccepted(ctx, evt)
}

func (m *PipelineManager) OnPartnerOfferExpired(ctx context.Context, evt events.PartnerOfferExpired) {
	m.orchestrator.OnPartnerOfferExpired(ctx, evt)
}

func (m *PipelineManager) OnPartnerOfferDeleted(ctx context.Context, evt events.PartnerOfferDeleted) {
	m.orchestrator.OnPartnerOfferDeleted(ctx, evt)
}

func (m *PipelineManager) OnPipelineStageChanged(ctx context.Context, evt events.PipelineStageChanged) {
	m.orchestrator.OnStageChange(ctx, evt)
}

func subscribePipelineManager(eventBus events.Bus, manager *PipelineManager) {
	subscribePipelineManagerLeadAutoDisqualified(eventBus, manager)
	subscribePipelineManagerQuoteAccepted(eventBus, manager)
	subscribePipelineManagerQuoteRejected(eventBus, manager)
	subscribePipelineManagerQuoteSent(eventBus, manager)
	subscribePipelineManagerPartnerOfferCreated(eventBus, manager)
	subscribePipelineManagerPartnerOfferRejected(eventBus, manager)
	subscribePipelineManagerPartnerOfferAccepted(eventBus, manager)
	subscribePipelineManagerPartnerOfferExpired(eventBus, manager)
	subscribePipelineManagerPartnerOfferDeleted(eventBus, manager)
	subscribePipelineManagerPipelineStageChanged(eventBus, manager)
}

func subscribePipelineManagerLeadAutoDisqualified(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.LeadAutoDisqualified{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadAutoDisqualified)
		if !ok {
			return nil
		}
		manager.OnLeadAutoDisqualified(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerQuoteAccepted(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.QuoteAccepted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteAccepted)
		if !ok {
			return nil
		}
		manager.OnQuoteAccepted(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerQuoteRejected(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.QuoteRejected{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteRejected)
		if !ok {
			return nil
		}
		manager.OnQuoteRejected(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerQuoteSent(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.QuoteSent{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteSent)
		if !ok {
			return nil
		}
		manager.OnQuoteSent(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPartnerOfferCreated(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PartnerOfferCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferCreated)
		if !ok {
			return nil
		}
		manager.OnPartnerOfferCreated(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPartnerOfferRejected(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PartnerOfferRejected{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferRejected)
		if !ok {
			return nil
		}
		manager.OnPartnerOfferRejected(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPartnerOfferAccepted(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PartnerOfferAccepted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferAccepted)
		if !ok {
			return nil
		}
		manager.OnPartnerOfferAccepted(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPartnerOfferExpired(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PartnerOfferExpired{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferExpired)
		if !ok {
			return nil
		}
		manager.OnPartnerOfferExpired(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPartnerOfferDeleted(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PartnerOfferDeleted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferDeleted)
		if !ok {
			return nil
		}
		manager.OnPartnerOfferDeleted(ctx, e)
		return nil
	}))
}

func subscribePipelineManagerPipelineStageChanged(eventBus events.Bus, manager *PipelineManager) {
	eventBus.Subscribe(events.PipelineStageChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PipelineStageChanged)
		if !ok {
			return nil
		}
		manager.OnPipelineStageChanged(ctx, e)
		return nil
	}))
}
