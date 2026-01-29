package events

import (
	"context"
	"sync"

	"portal_final_backend/internal/logger"
)

// InMemoryBus is an in-memory implementation of the event Bus interface.
// It executes handlers asynchronously by default for non-blocking event publishing.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	log      *logger.Logger
}

// NewInMemoryBus creates a new in-memory event bus.
func NewInMemoryBus(log *logger.Logger) *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[string][]Handler),
		log:      log,
	}
}

// Publish sends an event to all registered handlers asynchronously.
// Errors are logged but do not propagate back to the publisher.
func (b *InMemoryBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := b.handlers[event.EventName()]
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	// Execute all handlers asynchronously
	for _, h := range handlers {
		go func(handler Handler) {
			if err := handler.Handle(ctx, event); err != nil {
				b.log.Error("event handler failed",
					"event", event.EventName(),
					"error", err,
				)
			}
		}(h)
	}
}

// PublishSync sends an event and waits for all handlers to complete.
// Returns the first error encountered, but all handlers are still executed.
func (b *InMemoryBus) PublishSync(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.EventName()]
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(handlers))

	for _, h := range handlers {
		wg.Add(1)
		go func(handler Handler) {
			defer wg.Done()
			if err := handler.Handle(ctx, event); err != nil {
				errChan <- err
				b.log.Error("event handler failed",
					"event", event.EventName(),
					"error", err,
				)
			}
		}(h)
	}

	wg.Wait()
	close(errChan)

	// Return first error if any
	for err := range errChan {
		return err
	}

	return nil
}

// Subscribe registers a handler for a specific event type.
func (b *InMemoryBus) Subscribe(eventName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventName] = append(b.handlers[eventName], handler)
	b.log.Debug("event handler subscribed", "event", eventName)
}

// SubscribeFunc is a convenience method to subscribe a function as a handler.
func (b *InMemoryBus) SubscribeFunc(eventName string, fn func(ctx context.Context, event Event) error) {
	b.Subscribe(eventName, HandlerFunc(fn))
}

// Ensure InMemoryBus implements Bus
var _ Bus = (*InMemoryBus)(nil)
