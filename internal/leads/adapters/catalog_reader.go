package adapters

import (
	"context"
	"fmt"
	"strings"

	catalogrepo "portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// CatalogReaderAdapter implements ports.CatalogReader using the catalog repository.
//
// Important: draft products are intentionally excluded so AI search/quote drafting
// never uses incomplete placeholder pricing.
type CatalogReaderAdapter struct {
	repo catalogrepo.Repository
}

func NewCatalogReaderAdapter(repo catalogrepo.Repository) *CatalogReaderAdapter {
	return &CatalogReaderAdapter{repo: repo}
}

func (a *CatalogReaderAdapter) GetProductDetails(ctx context.Context, orgID uuid.UUID, productIDs []uuid.UUID) ([]ports.CatalogProductDetails, error) {
	if a == nil || a.repo == nil {
		return nil, fmt.Errorf("catalog reader not configured")
	}
	if len(productIDs) == 0 {
		return nil, nil
	}

	// Batch fetch products first.
	products, err := a.repo.GetProductsByIDs(ctx, orgID, productIDs)
	if err != nil {
		return nil, fmt.Errorf("get products by ids: %w", err)
	}

	out := make([]ports.CatalogProductDetails, 0, len(products))
	for _, p := range products {
		detail, err := a.getProductDetail(ctx, orgID, p)
		if err != nil {
			return nil, err
		}
		if detail != nil {
			out = append(out, *detail)
		}
	}

	return out, nil
}

func (a *CatalogReaderAdapter) getProductDetail(ctx context.Context, orgID uuid.UUID, p catalogrepo.Product) (*ports.CatalogProductDetails, error) {
	if p.IsDraft {
		// Draft products must never be used for AI material search / quote drafting.
		return nil, nil
	}

	// VAT rate: best-effort (non-fatal if missing).
	vatRateBps := 0
	if p.VatRateID != uuid.Nil {
		if rate, err := a.repo.GetVatRateByID(ctx, orgID, p.VatRateID); err == nil {
			vatRateBps = rate.RateBps
		}
	}

	materials, err := a.repo.ListProductMaterials(ctx, orgID, p.ID)
	if err != nil {
		return nil, fmt.Errorf("list product materials: %w", err)
	}
	materialNames := make([]string, 0, len(materials))
	for _, m := range materials {
		title := strings.TrimSpace(m.Title)
		if title == "" {
			continue
		}
		materialNames = append(materialNames, title)
	}

	docs, err := a.repo.ListProductAssets(ctx, catalogrepo.ListProductAssetsParams{
		OrganizationID: orgID,
		ProductID:      p.ID,
		AssetType:      strPtr("document"),
	})
	if err != nil {
		return nil, fmt.Errorf("list product document assets: %w", err)
	}

	urls, err := a.repo.ListProductAssets(ctx, catalogrepo.ListProductAssetsParams{
		OrganizationID: orgID,
		ProductID:      p.ID,
		AssetType:      strPtr("terms_url"),
	})
	if err != nil {
		return nil, fmt.Errorf("list product url assets: %w", err)
	}

	return &ports.CatalogProductDetails{
		ID:             p.ID,
		Title:          p.Title,
		Description:    derefString(p.Description),
		UnitPriceCents: p.UnitPriceCents,
		UnitLabel:      derefString(p.UnitLabel),
		LaborTimeText:  derefString(p.LaborTimeText),
		VatRateBps:     vatRateBps,
		Materials:      materialNames,
		Documents:      toCatalogDocuments(docs),
		URLs:           toCatalogURLs(urls),
	}, nil
}

func strPtr(s string) *string { return &s }

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toCatalogDocuments(assets []catalogrepo.ProductAsset) []ports.CatalogDocument {
	out := make([]ports.CatalogDocument, 0, len(assets))
	for _, d := range assets {
		if d.FileKey == nil || d.FileName == nil {
			continue
		}
		out = append(out, ports.CatalogDocument{
			ID:       d.ID,
			Filename: *d.FileName,
			FileKey:  *d.FileKey,
		})
	}
	return out
}

func toCatalogURLs(assets []catalogrepo.ProductAsset) []ports.CatalogURL {
	out := make([]ports.CatalogURL, 0, len(assets))
	for _, u := range assets {
		if u.URL == nil {
			continue
		}
		label := "Voorwaarden"
		if u.FileName != nil && strings.TrimSpace(*u.FileName) != "" {
			label = strings.TrimSpace(*u.FileName)
		}
		out = append(out, ports.CatalogURL{
			Label: label,
			Href:  *u.URL,
		})
	}
	return out
}
