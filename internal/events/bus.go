// Package events re-exports the platform event bus for convenience.
// This allows internal modules to import events from internal/events
// while the implementation lives in platform/events.
package events

import (
	platformevents "portal_final_backend/platform/events"
	"portal_final_backend/platform/logger"
)

// InMemoryBus is a type alias to the platform InMemoryBus
type InMemoryBus = platformevents.InMemoryBus

// NewInMemoryBus creates a new in-memory event bus.
// This is a convenience re-export from platform/events.
func NewInMemoryBus(log *logger.Logger) *InMemoryBus {
	return platformevents.NewInMemoryBus(log)
}
