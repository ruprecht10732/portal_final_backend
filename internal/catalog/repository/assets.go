package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	catalogdb "portal_final_backend/internal/catalog/db"
	"portal_final_backend/platform/apperr"
)

// Clean Code: Prefixed with errMsg to clarify intent and standardized naming.
const errMsgProductAssetNotFound = "product asset not found"

// CreateProductAsset creates a catalog product asset.
func (r *Repo) CreateProductAsset(ctx context.Context, params CreateProductAssetParams) (ProductAsset, error) {
	row, err := r.queries.CreateProductAsset(ctx, catalogdb.CreateProductAssetParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		ProductID:      toPgUUID(params.ProductID),
		AssetType:      params.AssetType,
		FileKey:        toPgText(params.FileKey),
		FileName:       toPgText(params.FileName),
		ContentType:    toPgText(params.ContentType),
		SizeBytes:      toPgInt8(params.SizeBytes),
		Url:            toPgText(params.URL),
	})
	if err != nil {
		return ProductAsset{}, fmt.Errorf("create product asset: %w", err)
	}

	return productAssetFromRow(row), nil
}

// GetProductAssetByID retrieves a product asset by ID.
// Security: Enforces multi-tenancy by mandating OrganizationID (prevents IDOR vulnerabilities).
func (r *Repo) GetProductAssetByID(ctx context.Context, organizationID, id uuid.UUID) (ProductAsset, error) {
	row, err := r.queries.GetProductAssetByID(ctx, catalogdb.GetProductAssetByIDParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductAsset{}, apperr.NotFound(errMsgProductAssetNotFound)
		}
		return ProductAsset{}, fmt.Errorf("get product asset by id: %w", err)
	}

	return productAssetFromRow(row), nil
}

// ListProductAssets lists assets for a product with an optional type filter.
//
// Tech Debt (Big O / Reliability): This query currently executes in O(N) space and time.
// Without database-level pagination (Limit/Offset), fetching a product with thousands
// of assets will cause high memory allocation spikes and potential DoS vectors.
// Consider adding pagination constraints to ListProductAssetsParams in v2.
func (r *Repo) ListProductAssets(ctx context.Context, params ListProductAssetsParams) ([]ProductAsset, error) {
	rows, err := r.queries.ListProductAssets(ctx, catalogdb.ListProductAssetsParams{
		Organizationid: toPgUUID(params.OrganizationID),
		Productid:      toPgUUID(params.ProductID),
		Assettype:      toPgText(params.AssetType),
	})
	if err != nil {
		return nil, fmt.Errorf("list product assets: %w", err)
	}

	// Performance: O(1) capacity allocation prevents the Go runtime from constantly
	// resizing and copying the underlying array during the loop (saving CPU cycles and GC pressure).
	items := make([]ProductAsset, 0, len(rows))
	for _, row := range rows {
		items = append(items, productAssetFromRow(row))
	}

	return items, nil
}

// DeleteProductAsset deletes a product asset by ID.
// Security: Enforces multi-tenancy by mandating OrganizationID.
func (r *Repo) DeleteProductAsset(ctx context.Context, organizationID, id uuid.UUID) error {
	rowsAffected, err := r.queries.DeleteProductAsset(ctx, catalogdb.DeleteProductAssetParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("delete product asset: %w", err)
	}
	if rowsAffected == 0 {
		return apperr.NotFound(errMsgProductAssetNotFound)
	}

	return nil
}

// productAssetFromRow maps the DB layer struct to the Bounded Context Domain struct.
func productAssetFromRow(row catalogdb.RacCatalogProductAsset) ProductAsset {
	return ProductAsset{
		ID:             row.ID.Bytes,
		OrganizationID: row.OrganizationID.Bytes,
		ProductID:      row.ProductID.Bytes,
		AssetType:      row.AssetType,
		FileKey:        optionalString(row.FileKey),
		FileName:       optionalString(row.FileName),
		ContentType:    optionalString(row.ContentType),
		SizeBytes:      optionalInt64(row.SizeBytes),
		URL:            optionalString(row.Url),
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
	}
}

// optionalInt64 converts pgtype.Int8 to a *int64.
func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	// Clean Code: Removed intermediate variable assignment.
	return &value.Int64
}
