// Package energylabel defines the public contract for energy label integration.
// Only types and interfaces defined here should be imported by external domains
// to maintain strict Bounded Context isolation.
package energylabel

import (
	"context"

	"portal_final_backend/internal/energylabel/transport"
)

// Service defines the public API for energy label lookups.
// Implementation must ensure all upstream calls are protected by the provided context
// to prevent goroutine leaks and ensure O(1) response latency (relative to timeout).
type Service interface {
	// GetByAddress fetches the energy label for a specific Dutch address.
	// Returns (nil, nil) if the address is valid but no label exists.
	// Returns an error only on transport failures or upstream API unavailability.
	GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) (*transport.EnergyLabel, error)

	// GetByBAGObjectID fetches a label using a BAG adresseerbaar object ID.
	// Expected to be used when address components are already normalized.
	GetByBAGObjectID(ctx context.Context, objectID string) (*transport.EnergyLabel, error)

	// Ping validates the availability of the underlying EP-Online infrastructure.
	// Use this for readiness probes or health check aggregation.
	Ping(ctx context.Context) error
}

// Ensure implementation adheres to the contract at compile time.
// Note: Concrete implementation should be initialized in the module.go using:
// var _ energylabel.Service = (*service.Service)(nil)
