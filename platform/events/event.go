// Package events provides event bus infrastructure for decoupled,
// event-driven communication between modules.
// This is part of the platform layer and contains no business logic.
package events

import (
	"context"
	"time"
)

// Event is the base interface all domain events must implement.
type Event interface {
	// EventName returns a unique identifier for the event type.
	EventName() string
	// OccurredAt returns when the event occurred.
	OccurredAt() time.Time
}

// BaseEvent provides common fields for all events.
type BaseEvent struct {
	Timestamp time.Time `json:"timestamp"`
}

// OccurredAt returns when the event occurred.
func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// NewBaseEvent creates a new base event with the current timestamp.
func NewBaseEvent() BaseEvent {
	return BaseEvent{Timestamp: time.Now()}
}

// Handler processes events of a specific type.
type Handler interface {
	Handle(ctx context.Context, event Event) error
}

// HandlerFunc is an adapter to allow ordinary functions to be used as handlers.
type HandlerFunc func(ctx context.Context, event Event) error

// Handle calls the underlying function.
func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Bus is the interface for publishing and subscribing to domain events.
type Bus interface {
	// Publish sends an event to all registered handlers for that event type.
	// Handlers are executed asynchronously by default.
	Publish(ctx context.Context, event Event)

	// PublishSync sends an event and waits for all handlers to complete.
	PublishSync(ctx context.Context, event Event) error

	// Subscribe registers a handler for a specific event type.
	// The eventName should match the value returned by Event.EventName().
	Subscribe(eventName string, handler Handler)
}
