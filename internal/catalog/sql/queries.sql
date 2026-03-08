-- Catalog Domain SQL Queries

-- VAT Rates

-- name: CreateVatRate :one
INSERT INTO RAC_catalog_vat_rates (organization_id, name, rate_bps)
VALUES ($1, $2, $3)
RETURNING id, organization_id, name, rate_bps, created_at, updated_at;

-- name: GetVatRateByID :one
SELECT id, organization_id, name, rate_bps, created_at, updated_at
FROM RAC_catalog_vat_rates
WHERE id = $1 AND organization_id = $2;

-- name: ListVatRates :many
SELECT id, organization_id, name, rate_bps, created_at, updated_at
FROM RAC_catalog_vat_rates
WHERE organization_id = sqlc.arg(organizationID)
  AND (sqlc.narg(searchPattern)::text IS NULL OR name ILIKE sqlc.narg(searchPattern)::text)
ORDER BY
  CASE WHEN sqlc.arg(sortBy) = 'name' AND sqlc.arg(sortOrder) = 'asc' THEN name END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'name' AND sqlc.arg(sortOrder) = 'desc' THEN name END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'rateBps' AND sqlc.arg(sortOrder) = 'asc' THEN rate_bps END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'rateBps' AND sqlc.arg(sortOrder) = 'desc' THEN rate_bps END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'createdAt' AND sqlc.arg(sortOrder) = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'createdAt' AND sqlc.arg(sortOrder) = 'desc' THEN created_at END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'updatedAt' AND sqlc.arg(sortOrder) = 'asc' THEN updated_at END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'updatedAt' AND sqlc.arg(sortOrder) = 'desc' THEN updated_at END DESC,
  name ASC
LIMIT sqlc.arg(limitCount) OFFSET sqlc.arg(offsetCount);

-- name: CountVatRates :one
SELECT COUNT(*) AS countValue
FROM RAC_catalog_vat_rates
WHERE organization_id = sqlc.arg(organizationID)
  AND (sqlc.narg(searchPattern)::text IS NULL OR name ILIKE sqlc.narg(searchPattern)::text);

-- name: UpdateVatRate :one
UPDATE RAC_catalog_vat_rates
SET
  name = COALESCE(sqlc.narg(name), name),
  rate_bps = COALESCE(sqlc.narg(rateBps), rate_bps),
  updated_at = now()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organizationID)
RETURNING id, organization_id, name, rate_bps, created_at, updated_at;

-- name: DeleteVatRate :execrows
DELETE FROM RAC_catalog_vat_rates
WHERE id = $1 AND organization_id = $2;

-- name: HasProductsWithVatRate :one
SELECT EXISTS(SELECT 1 FROM RAC_catalog_products WHERE vat_rate_id = $1 AND organization_id = $2);

-- Products

-- name: CreateProduct :one
INSERT INTO RAC_catalog_products (
  organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit,
  created_at, updated_at;

-- name: GetNextProductCounter :one
INSERT INTO RAC_catalog_product_counters (organization_id, last_number)
VALUES ($1, 1)
ON CONFLICT (organization_id) DO UPDATE
SET last_number = RAC_catalog_product_counters.last_number + 1
RETURNING last_number AS lastNumberValue;

-- name: UpdateProduct :one
UPDATE RAC_catalog_products
SET
  vat_rate_id = COALESCE(sqlc.narg(vatRateID), vat_rate_id),
  is_draft = COALESCE(sqlc.narg(isDraft), is_draft),
  title = COALESCE(sqlc.narg(title), title),
  reference = COALESCE(sqlc.narg(reference), reference),
  description = COALESCE(sqlc.narg(description), description),
  price_cents = COALESCE(sqlc.narg(priceCents), price_cents),
  unit_price_cents = COALESCE(sqlc.narg(unitPriceCents), unit_price_cents),
  unit_label = COALESCE(sqlc.narg(unitLabel), unit_label),
  labor_time_text = COALESCE(sqlc.narg(laborTimeText), labor_time_text),
  type = COALESCE(sqlc.narg(type), type),
  period_count = COALESCE(sqlc.narg(periodCount), period_count),
  period_unit = COALESCE(sqlc.narg(periodUnit), period_unit),
  updated_at = now()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organizationID)
RETURNING id, organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit,
  created_at, updated_at;

-- name: DeleteProduct :execrows
DELETE FROM RAC_catalog_products
WHERE id = $1 AND organization_id = $2;

-- name: GetProductByID :one
SELECT id, organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit,
  created_at, updated_at
FROM RAC_catalog_products
WHERE id = $1 AND organization_id = $2;

-- name: ListProducts :many
SELECT id, organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit,
  created_at, updated_at
FROM RAC_catalog_products
WHERE organization_id = sqlc.arg(organizationID)
  AND (sqlc.narg(searchPattern)::text IS NULL OR (title ILIKE sqlc.narg(searchPattern)::text OR reference ILIKE sqlc.narg(searchPattern)::text))
  AND (sqlc.narg(titlePattern)::text IS NULL OR title ILIKE sqlc.narg(titlePattern)::text)
  AND (sqlc.narg(referencePattern)::text IS NULL OR reference ILIKE sqlc.narg(referencePattern)::text)
  AND (sqlc.narg(productType)::text IS NULL OR type = sqlc.narg(productType)::text)
  AND (sqlc.narg(isDraft)::bool IS NULL OR is_draft = sqlc.narg(isDraft)::bool)
  AND (sqlc.narg(vatRateID)::uuid IS NULL OR vat_rate_id = sqlc.narg(vatRateID)::uuid)
  AND (sqlc.narg(createdAtFrom)::timestamptz IS NULL OR created_at >= sqlc.narg(createdAtFrom)::timestamptz)
  AND (sqlc.narg(createdAtTo)::timestamptz IS NULL OR created_at <= sqlc.narg(createdAtTo)::timestamptz)
  AND (sqlc.narg(updatedAtFrom)::timestamptz IS NULL OR updated_at >= sqlc.narg(updatedAtFrom)::timestamptz)
  AND (sqlc.narg(updatedAtTo)::timestamptz IS NULL OR updated_at <= sqlc.narg(updatedAtTo)::timestamptz)
ORDER BY
  CASE WHEN sqlc.arg(sortBy) = 'title' AND sqlc.arg(sortOrder) = 'asc' THEN title END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'title' AND sqlc.arg(sortOrder) = 'desc' THEN title END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'reference' AND sqlc.arg(sortOrder) = 'asc' THEN reference END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'reference' AND sqlc.arg(sortOrder) = 'desc' THEN reference END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'priceCents' AND sqlc.arg(sortOrder) = 'asc' THEN price_cents END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'priceCents' AND sqlc.arg(sortOrder) = 'desc' THEN price_cents END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'type' AND sqlc.arg(sortOrder) = 'asc' THEN type END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'type' AND sqlc.arg(sortOrder) = 'desc' THEN type END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'isDraft' AND sqlc.arg(sortOrder) = 'asc' THEN is_draft END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'isDraft' AND sqlc.arg(sortOrder) = 'desc' THEN is_draft END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'vatRateId' AND sqlc.arg(sortOrder) = 'asc' THEN vat_rate_id END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'vatRateId' AND sqlc.arg(sortOrder) = 'desc' THEN vat_rate_id END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'createdAt' AND sqlc.arg(sortOrder) = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'createdAt' AND sqlc.arg(sortOrder) = 'desc' THEN created_at END DESC,
  CASE WHEN sqlc.arg(sortBy) = 'updatedAt' AND sqlc.arg(sortOrder) = 'asc' THEN updated_at END ASC,
  CASE WHEN sqlc.arg(sortBy) = 'updatedAt' AND sqlc.arg(sortOrder) = 'desc' THEN updated_at END DESC,
  created_at DESC
LIMIT sqlc.arg(limitCount) OFFSET sqlc.arg(offsetCount);

-- name: CountProducts :one
SELECT COUNT(*) AS countValue
FROM RAC_catalog_products
WHERE organization_id = sqlc.arg(organizationID)
  AND (sqlc.narg(searchPattern)::text IS NULL OR (title ILIKE sqlc.narg(searchPattern)::text OR reference ILIKE sqlc.narg(searchPattern)::text))
  AND (sqlc.narg(titlePattern)::text IS NULL OR title ILIKE sqlc.narg(titlePattern)::text)
  AND (sqlc.narg(referencePattern)::text IS NULL OR reference ILIKE sqlc.narg(referencePattern)::text)
  AND (sqlc.narg(productType)::text IS NULL OR type = sqlc.narg(productType)::text)
  AND (sqlc.narg(isDraft)::bool IS NULL OR is_draft = sqlc.narg(isDraft)::bool)
  AND (sqlc.narg(vatRateID)::uuid IS NULL OR vat_rate_id = sqlc.narg(vatRateID)::uuid)
  AND (sqlc.narg(createdAtFrom)::timestamptz IS NULL OR created_at >= sqlc.narg(createdAtFrom)::timestamptz)
  AND (sqlc.narg(createdAtTo)::timestamptz IS NULL OR created_at <= sqlc.narg(createdAtTo)::timestamptz)
  AND (sqlc.narg(updatedAtFrom)::timestamptz IS NULL OR updated_at >= sqlc.narg(updatedAtFrom)::timestamptz)
  AND (sqlc.narg(updatedAtTo)::timestamptz IS NULL OR updated_at <= sqlc.narg(updatedAtTo)::timestamptz);

-- name: GetProductsByIDs :many
SELECT id, organization_id, vat_rate_id, is_draft,
  title, reference, description,
  price_cents, unit_price_cents, unit_label, labor_time_text,
  type, period_count, period_unit,
  created_at, updated_at
FROM RAC_catalog_products
WHERE organization_id = sqlc.arg(organizationID)
  AND id = ANY(sqlc.arg(productIDs)::uuid[]);

-- Product Materials

-- name: UpsertProductMaterial :exec
INSERT INTO RAC_catalog_product_materials (organization_id, product_id, material_id, pricing_mode)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, product_id, material_id)
DO UPDATE SET pricing_mode = EXCLUDED.pricing_mode;

-- name: RemoveProductMaterials :exec
DELETE FROM RAC_catalog_product_materials
WHERE organization_id = $1 AND product_id = $2 AND material_id = ANY($3::uuid[]);

-- name: ListProductMaterials :many
SELECT p.id, p.organization_id, p.vat_rate_id, p.is_draft,
  p.title, p.reference, p.description,
  p.price_cents, p.unit_price_cents, p.unit_label, p.labor_time_text,
  p.type, pm.pricing_mode, p.period_count, p.period_unit,
  p.created_at, p.updated_at
FROM RAC_catalog_products p
JOIN RAC_catalog_product_materials pm
  ON pm.material_id = p.id AND pm.organization_id = p.organization_id
WHERE pm.organization_id = $1 AND pm.product_id = $2
ORDER BY p.title ASC;

-- name: HasProductMaterials :one
SELECT EXISTS(SELECT 1 FROM RAC_catalog_product_materials WHERE organization_id = $1 AND product_id = $2);

-- Product Assets

-- name: CreateProductAsset :one
INSERT INTO RAC_catalog_product_assets (
  organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at;

-- name: GetProductAssetByID :one
SELECT id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at
FROM RAC_catalog_product_assets
WHERE id = $1 AND organization_id = $2;

-- name: ListProductAssets :many
SELECT id, organization_id, product_id, asset_type, file_key, file_name, content_type, size_bytes, url, created_at
FROM RAC_catalog_product_assets
WHERE organization_id = sqlc.arg(organizationID)
  AND product_id = sqlc.arg(productID)
  AND (sqlc.narg(assetType)::text IS NULL OR asset_type = sqlc.narg(assetType)::text)
ORDER BY created_at DESC;

-- name: DeleteProductAsset :execrows
DELETE FROM RAC_catalog_product_assets
WHERE id = $1 AND organization_id = $2;