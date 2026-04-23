// Package events provides a domain-specific entry point for the system's event infrastructure.
// By re-exporting the platform implementation, we maintain a clean 'internal' boundary
// while allowing the core logic to remain in a reusable 'platform' layer.
package events

import (
	platformevents "portal_final_backend/platform/events"
	"portal_final_backend/platform/logger"
)

// InMemoryBus is a Type Alias to the platform's event bus implementation.
//
// Principal Note: Using an Alias (=) instead of a defined type ensures that
// internal modules can pass this bus to platform functions without explicit
// type conversion, maintaining O(1) compiler compatibility.
type InMemoryBus = platformevents.InMemoryBus

// NewInMemoryBus initializes a high-performance, synchronous in-memory event bus.
//
// Complexity: O(1) initialization.
// Thread-Safety: Implementation in platform/events handles internal synchronization.
func NewInMemoryBus(log *logger.Logger) *InMemoryBus {
	// We delegate directly to the platform constructor.
	// This wrapper ensures that if the platform API changes, we only update this
	// single internal facade rather than every module in the project.
	return platformevents.NewInMemoryBus(log)
}
