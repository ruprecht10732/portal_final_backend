package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	catrepo "portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/leads/ports"
)

// CatalogProductReader adapts the catalog repository for the leads domain.
// It composes GetProductsByIDs, ListProductMaterials and GetVatRateByID to
// return fully-hydrated CatalogProductDetails, satisfying ports.CatalogReader.
type CatalogProductReader struct {
	repo catrepo.Repository
}

// NewCatalogProductReader creates a new catalog reader adapter.
func NewCatalogProductReader(repo catrepo.Repository) *CatalogProductReader {
	return &CatalogProductReader{repo: repo}
}

// GetProductDetails returns enriched product details (including materials and
// resolved VAT rate) for the given IDs. Unknown IDs are silently omitted.
func (a *CatalogProductReader) GetProductDetails(ctx context.Context, orgID uuid.UUID, productIDs []uuid.UUID) ([]ports.CatalogProductDetails, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}

	products, err := a.repo.GetProductsByIDs(ctx, orgID, productIDs)
	if err != nil {
		return nil, fmt.Errorf("catalog adapter: get products: %w", err)
	}

	// Collect unique VAT rate IDs.
	vatIDs := make(map[uuid.UUID]struct{})
	for _, p := range products {
		vatIDs[p.VatRateID] = struct{}{}
	}

	// Batch-fetch VAT rates.
	vatRates := make(map[uuid.UUID]int) // vatRateID → rateBps
	for id := range vatIDs {
		vr, err := a.repo.GetVatRateByID(ctx, orgID, id)
		if err != nil {
			// Non-fatal: default to 0 if VAT rate lookup fails.
			continue
		}
		vatRates[id] = vr.RateBps
	}

	result := make([]ports.CatalogProductDetails, 0, len(products))
	for _, p := range products {
		result = append(result, a.productToDetail(ctx, orgID, p, vatRates))
	}

	return result, nil
}

// productToDetail converts a single catalog product into a CatalogProductDetails,
// enriching it with materials and the resolved VAT rate.
func (a *CatalogProductReader) productToDetail(ctx context.Context, orgID uuid.UUID, p catrepo.Product, vatRates map[uuid.UUID]int) ports.CatalogProductDetails {
	materials, err := a.repo.ListProductMaterials(ctx, orgID, p.ID)
	if err != nil {
		materials = nil
	}

	matNames := make([]string, len(materials))
	for i, m := range materials {
		matNames[i] = m.Title
	}

	detail := ports.CatalogProductDetails{
		ID:             p.ID,
		Title:          p.Title,
		UnitPriceCents: p.UnitPriceCents,
		VatRateBps:     vatRates[p.VatRateID],
		Materials:      matNames,
	}

	if p.Description != nil {
		detail.Description = *p.Description
	}
	if p.UnitLabel != nil {
		detail.UnitLabel = *p.UnitLabel
	}
	if p.LaborTimeText != nil {
		detail.LaborTimeText = *p.LaborTimeText
	}

	return detail
}

// Compile-time check that CatalogProductReader implements ports.CatalogReader.
var _ ports.CatalogReader = (*CatalogProductReader)(nil)
