package leads

import (
	"context"

	"portal_final_backend/internal/events"
)

// subscribeOrchestratorEvents wires all orchestrator event handlers directly to the event bus,
// eliminating the pointless PipelineManager / StateReconciler / AgentCoordinator proxy layers.
func subscribeOrchestratorEvents(eventBus events.Bus, o *Orchestrator) {
	// Pipeline events
	eventBus.Subscribe(events.LeadAutoDisqualified{}.EventName(), typedHandler(o.OnLeadAutoDisqualified))
	eventBus.Subscribe(events.QuoteAccepted{}.EventName(), typedHandler(o.OnQuoteAccepted))
	eventBus.Subscribe(events.QuoteRejected{}.EventName(), typedHandler(o.OnQuoteRejected))
	eventBus.Subscribe(events.QuoteSent{}.EventName(), typedHandler(o.OnQuoteSent))
	eventBus.Subscribe(events.PartnerOfferCreated{}.EventName(), typedHandler(o.OnPartnerOfferCreated))
	eventBus.Subscribe(events.PartnerOfferRejected{}.EventName(), typedHandler(o.OnPartnerOfferRejected))
	eventBus.Subscribe(events.PartnerOfferAccepted{}.EventName(), typedHandler(o.OnPartnerOfferAccepted))
	eventBus.Subscribe(events.PartnerOfferExpired{}.EventName(), typedHandler(o.OnPartnerOfferExpired))
	eventBus.Subscribe(events.PartnerOfferDeleted{}.EventName(), typedHandler(o.OnPartnerOfferDeleted))
	eventBus.Subscribe(events.PipelineStageChanged{}.EventName(), typedHandler(func(ctx context.Context, evt events.PipelineStageChanged) {
		o.OnStageChange(ctx, evt)
	}))

	// State reconciliation events
	eventBus.Subscribe(events.QuoteCreated{}.EventName(), typedHandler(o.OnQuoteCreated))
	eventBus.Subscribe(events.QuoteDeleted{}.EventName(), typedHandler(o.OnQuoteDeleted))
	eventBus.Subscribe(events.AppointmentCreated{}.EventName(), typedHandler(o.OnAppointmentCreated))
	eventBus.Subscribe(events.AppointmentStatusChanged{}.EventName(), typedHandler(o.OnAppointmentStatusChanged))
	eventBus.Subscribe(events.AppointmentDeleted{}.EventName(), typedHandler(o.OnAppointmentDeleted))
	eventBus.Subscribe(events.QuoteStatusChanged{}.EventName(), typedHandler(o.OnQuoteStatusChanged))
	eventBus.Subscribe(events.LeadServiceStatusChanged{}.EventName(), typedHandler(o.OnLeadServiceStatusChanged))

	// Agent trigger events
	eventBus.Subscribe(events.LeadDataChanged{}.EventName(), typedHandler(func(ctx context.Context, evt events.LeadDataChanged) {
		o.OnDataChange(ctx, evt)
	}))
	eventBus.Subscribe(events.VisitReportSubmitted{}.EventName(), typedHandler(o.OnVisitReportSubmitted))
}

// typedHandler wraps a typed event handler into the events.HandlerFunc interface.
func typedHandler[T any](fn func(context.Context, T)) events.HandlerFunc {
	return func(ctx context.Context, event events.Event) error {
		e, ok := event.(T)
		if !ok {
			return nil
		}
		fn(ctx, e)
		return nil
	}
}
