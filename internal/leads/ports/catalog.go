// Package ports defines the interfaces that the RAC_leads domain requires from
// external systems. These interfaces form the Anti-Corruption Layer (ACL),
// ensuring the RAC_leads domain only knows about the data it needs, formatted
// the way it wants.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// CatalogProductDetails holds the product information the leads/agent needs for
// quote drafting. Materials are pre-resolved to simple description strings so
// the agent never touches the catalog domain directly.
type CatalogProductDetails struct {
	ID             uuid.UUID
	Title          string
	Description    string
	UnitPriceCents int64
	UnitLabel      string
	LaborTimeText  string
	VatRateBps     int
	Materials      []string // human-readable material names
}

// CatalogReader is the ACL interface through which the leads domain can look up
// catalog product details (including materials). The adapter composes existing
// catalog repository methods under the hood.
type CatalogReader interface {
	// GetProductDetails returns enriched product details for the given IDs.
	// Unknown IDs are silently omitted from the result slice.
	GetProductDetails(ctx context.Context, orgID uuid.UUID, productIDs []uuid.UUID) ([]CatalogProductDetails, error)
}
