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

	// Safety: never expose draft products to the leads AI flows.
	filtered := make([]catrepo.Product, 0, len(products))
	for _, p := range products {
		if p.IsDraft {
			continue
		}
		filtered = append(filtered, p)
	}
	products = filtered

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
// enriching it with materials, document/URL assets, and the resolved VAT rate.
func (a *CatalogProductReader) productToDetail(ctx context.Context, orgID uuid.UUID, p catrepo.Product, vatRates map[uuid.UUID]int) ports.CatalogProductDetails {
	// Catalog products use either price_cents (fixed price) or unit_price_cents
	// (per-unit price) — never both. Resolve to a single effective price so that
	// downstream consumers (AI search hydration and catalog price enforcement) always
	// receive the correct non-zero value regardless of which pricing mode is used.
	effectivePriceCents := p.UnitPriceCents
	if effectivePriceCents == 0 {
		effectivePriceCents = p.PriceCents
	}

	detail := ports.CatalogProductDetails{
		ID:             p.ID,
		Title:          p.Title,
		UnitPriceCents: effectivePriceCents,
		VatRateBps:     vatRates[p.VatRateID],
		Materials:      a.fetchMaterialNames(ctx, orgID, p.ID),
	}

	setOptional(&detail.Description, p.Description)
	setOptional(&detail.UnitLabel, p.UnitLabel)
	setOptional(&detail.LaborTimeText, p.LaborTimeText)

	detail.Documents = a.fetchDocumentAssets(ctx, orgID, p.ID)
	detail.URLs = a.fetchURLAssets(ctx, orgID, p.ID)

	return detail
}

// setOptional assigns src to dst when src is non-nil.
func setOptional(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

// fetchMaterialNames returns the material titles for a product.
func (a *CatalogProductReader) fetchMaterialNames(ctx context.Context, orgID, productID uuid.UUID) []string {
	materials, err := a.repo.ListProductMaterials(ctx, orgID, productID)
	if err != nil {
		return nil
	}
	names := make([]string, len(materials))
	for i, m := range materials {
		names[i] = m.Title
	}
	return names
}

// fetchDocumentAssets returns catalog document assets (PDFs, specs) for a product.
func (a *CatalogProductReader) fetchDocumentAssets(ctx context.Context, orgID, productID uuid.UUID) []ports.CatalogDocument {
	docType := "document"
	assets, err := a.repo.ListProductAssets(ctx, catrepo.ListProductAssetsParams{
		OrganizationID: orgID,
		ProductID:      productID,
		AssetType:      &docType,
	})
	if err != nil {
		return nil
	}
	var docs []ports.CatalogDocument
	for _, asset := range assets {
		if asset.FileKey != nil && asset.FileName != nil {
			docs = append(docs, ports.CatalogDocument{
				ID:       asset.ID,
				Filename: *asset.FileName,
				FileKey:  *asset.FileKey,
			})
		}
	}
	return docs
}

// fetchURLAssets returns catalog URL assets (terms & conditions, links) for a product.
func (a *CatalogProductReader) fetchURLAssets(ctx context.Context, orgID, productID uuid.UUID) []ports.CatalogURL {
	urlType := "terms_url"
	assets, err := a.repo.ListProductAssets(ctx, catrepo.ListProductAssetsParams{
		OrganizationID: orgID,
		ProductID:      productID,
		AssetType:      &urlType,
	})
	if err != nil {
		return nil
	}
	var urls []ports.CatalogURL
	for _, asset := range assets {
		if asset.URL == nil || *asset.URL == "" {
			continue
		}
		label := "Link"
		if asset.FileName != nil && *asset.FileName != "" {
			label = *asset.FileName
		}
		urls = append(urls, ports.CatalogURL{
			Label: label,
			Href:  *asset.URL,
		})
	}
	return urls
}

// Compile-time check that CatalogProductReader implements ports.CatalogReader.
var _ ports.CatalogReader = (*CatalogProductReader)(nil)
