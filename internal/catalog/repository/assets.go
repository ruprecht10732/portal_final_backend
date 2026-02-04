package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"portal_final_backend/platform/apperr"
)

const productAssetNotFoundMessage = "product asset not found"

// CreateProductAsset creates a catalog product asset.
func (r *Repo) CreateProductAsset(ctx context.Context, params CreateProductAssetParams) (ProductAsset, error) {
	query := `
        INSERT INTO catalog_product_assets (
            organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at`

	var asset ProductAsset
	var createdAt time.Time
	if err := r.pool.QueryRow(ctx, query,
		params.OrganizationID,
		params.ProductID,
		params.AssetType,
		params.FileKey,
		params.FileName,
		params.ContentType,
		params.SizeBytes,
		params.URL,
	).Scan(
		&asset.ID,
		&asset.OrganizationID,
		&asset.ProductID,
		&asset.AssetType,
		&asset.FileKey,
		&asset.FileName,
		&asset.ContentType,
		&asset.SizeBytes,
		&asset.URL,
		&createdAt,
	); err != nil {
		return ProductAsset{}, fmt.Errorf("create product asset: %w", err)
	}

	asset.CreatedAt = createdAt.Format(time.RFC3339)
	return asset, nil
}

// GetProductAssetByID retrieves a product asset by ID.
func (r *Repo) GetProductAssetByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (ProductAsset, error) {
	query := `
        SELECT id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at
        FROM catalog_product_assets
        WHERE id = $1 AND organization_id = $2`

	var asset ProductAsset
	var createdAt time.Time
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&asset.ID,
		&asset.OrganizationID,
		&asset.ProductID,
		&asset.AssetType,
		&asset.FileKey,
		&asset.FileName,
		&asset.ContentType,
		&asset.SizeBytes,
		&asset.URL,
		&createdAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductAsset{}, apperr.NotFound(productAssetNotFoundMessage)
		}
		return ProductAsset{}, fmt.Errorf("get product asset by id: %w", err)
	}

	asset.CreatedAt = createdAt.Format(time.RFC3339)
	return asset, nil
}

// ListProductAssets lists assets for a product with optional type filter.
func (r *Repo) ListProductAssets(ctx context.Context, params ListProductAssetsParams) ([]ProductAsset, error) {
	whereClause := "organization_id = $1 AND product_id = $2"
	args := []interface{}{params.OrganizationID, params.ProductID}

	if params.AssetType != nil {
		whereClause += " AND asset_type = $3"
		args = append(args, *params.AssetType)
	}

	query := fmt.Sprintf(`
        SELECT id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at
        FROM catalog_product_assets
        WHERE %s
        ORDER BY created_at DESC`, whereClause)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list product assets: %w", err)
	}
	defer rows.Close()

	items := make([]ProductAsset, 0)
	for rows.Next() {
		var asset ProductAsset
		var createdAt time.Time
		if err := rows.Scan(
			&asset.ID,
			&asset.OrganizationID,
			&asset.ProductID,
			&asset.AssetType,
			&asset.FileKey,
			&asset.FileName,
			&asset.ContentType,
			&asset.SizeBytes,
			&asset.URL,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan product asset: %w", err)
		}
		asset.CreatedAt = createdAt.Format(time.RFC3339)
		items = append(items, asset)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate product assets: %w", rows.Err())
	}

	return items, nil
}

// DeleteProductAsset deletes a product asset by ID.
func (r *Repo) DeleteProductAsset(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	query := `DELETE FROM catalog_product_assets WHERE id = $1 AND organization_id = $2`
	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete product asset: %w", err)
	}
	if result.RowsAffected() == 0 {
		return apperr.NotFound(productAssetNotFoundMessage)
	}
	return nil
}
