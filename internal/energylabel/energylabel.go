// Package energylabel provides the energy label bounded context.
// This file defines the public interfaces exposed to other domains.
package energylabel

import (
	"context"

	"portal_final_backend/internal/energylabel/transport"
)

// EnergyLabelService defines the public interface for energy label lookups.
// Other domains should depend on this interface, not the concrete implementation.
type EnergyLabelService interface {
	// GetByAddress fetches the energy label for a given Dutch address.
	// Returns nil if no label is found for the address.
	GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) (*transport.EnergyLabel, error)

	// GetByBAGObjectID fetches the energy label for a BAG adresseerbaar object ID.
	// Returns nil if no label is found for the object.
	GetByBAGObjectID(ctx context.Context, objectID string) (*transport.EnergyLabel, error)

	// Ping checks if the EP-Online API is available.
	Ping(ctx context.Context) error
}
