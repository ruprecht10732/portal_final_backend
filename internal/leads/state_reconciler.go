package leads

import (
	"context"

	"portal_final_backend/internal/events"
)

// StateReconciler keeps pipeline and lifecycle state consistent across quote/appointment events.
type StateReconciler struct {
	orchestrator *Orchestrator
}

func newStateReconciler(orchestrator *Orchestrator) *StateReconciler {
	return &StateReconciler{orchestrator: orchestrator}
}

func (r *StateReconciler) OnQuoteCreated(ctx context.Context, evt events.QuoteCreated) {
	r.orchestrator.OnQuoteCreated(ctx, evt)
}

func (r *StateReconciler) OnQuoteDeleted(ctx context.Context, evt events.QuoteDeleted) {
	r.orchestrator.OnQuoteDeleted(ctx, evt)
}

func (r *StateReconciler) OnAppointmentCreated(ctx context.Context, evt events.AppointmentCreated) {
	r.orchestrator.OnAppointmentCreated(ctx, evt)
}

func (r *StateReconciler) OnAppointmentStatusChanged(ctx context.Context, evt events.AppointmentStatusChanged) {
	r.orchestrator.OnAppointmentStatusChanged(ctx, evt)
}

func (r *StateReconciler) OnAppointmentDeleted(ctx context.Context, evt events.AppointmentDeleted) {
	r.orchestrator.OnAppointmentDeleted(ctx, evt)
}

func (r *StateReconciler) OnQuoteStatusChanged(ctx context.Context, evt events.QuoteStatusChanged) {
	r.orchestrator.OnQuoteStatusChanged(ctx, evt)
}

func (r *StateReconciler) OnLeadServiceStatusChanged(ctx context.Context, evt events.LeadServiceStatusChanged) {
	r.orchestrator.OnLeadServiceStatusChanged(ctx, evt)
}

func subscribeStateReconciler(eventBus events.Bus, reconciler *StateReconciler) {
	eventBus.Subscribe(events.QuoteCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteCreated)
		if !ok {
			return nil
		}
		reconciler.OnQuoteCreated(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.QuoteDeleted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteDeleted)
		if !ok {
			return nil
		}
		reconciler.OnQuoteDeleted(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.AppointmentCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.AppointmentCreated)
		if !ok {
			return nil
		}
		reconciler.OnAppointmentCreated(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.AppointmentStatusChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.AppointmentStatusChanged)
		if !ok {
			return nil
		}
		reconciler.OnAppointmentStatusChanged(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.AppointmentDeleted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.AppointmentDeleted)
		if !ok {
			return nil
		}
		reconciler.OnAppointmentDeleted(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.QuoteStatusChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteStatusChanged)
		if !ok {
			return nil
		}
		reconciler.OnQuoteStatusChanged(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.LeadServiceStatusChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadServiceStatusChanged)
		if !ok {
			return nil
		}
		reconciler.OnLeadServiceStatusChanged(ctx, e)
		return nil
	}))
}
