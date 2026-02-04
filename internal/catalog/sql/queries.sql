-- Catalog Domain SQL Queries

-- VAT Rates

-- name: CreateVatRate :one
INSERT INTO RAC_catalog_vat_rates (organization_id, name, rate_bps)
VALUES ($1, $2, $3)
RETURNING id, organization_id, name, rate_bps, created_at, updated_at;

-- name: GetVatRateByID :one
SELECT id, organization_id, name, rate_bps, created_at, updated_at FROM RAC_catalog_vat_rates
WHERE id = $1 AND organization_id = $2;

-- name: ListVatRates :many
SELECT id, organization_id, name, rate_bps, created_at, updated_at FROM RAC_catalog_vat_rates
WHERE organization_id = $1
  AND ($2 = '' OR name ILIKE $2)
ORDER BY name ASC
LIMIT $3 OFFSET $4;

-- name: CountVatRates :one
SELECT COUNT(*) FROM RAC_catalog_vat_rates
WHERE organization_id = $1
  AND ($2 = '' OR name ILIKE $2);

-- name: UpdateVatRate :one
UPDATE RAC_catalog_vat_rates
SET
  name = COALESCE($3, name),
  rate_bps = COALESCE($4, rate_bps),
  updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING id, organization_id, name, rate_bps, created_at, updated_at;

-- name: DeleteVatRate :exec
DELETE FROM RAC_catalog_vat_rates
WHERE id = $1 AND organization_id = $2;

-- name: HasProductsWithVatRate :one
SELECT EXISTS(SELECT 1 FROM RAC_catalog_products WHERE vat_rate_id = $1 AND organization_id = $2);

-- Products

-- name: CreateProduct :one
INSERT INTO RAC_catalog_products (
  organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit, created_at, updated_at;

-- name: GetProductByID :one
SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit, created_at, updated_at FROM RAC_catalog_products
WHERE id = $1 AND organization_id = $2;

-- name: ListProducts :many
SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit, created_at, updated_at FROM RAC_catalog_products
WHERE organization_id = $1
  AND ($2 = '' OR title ILIKE $2 OR reference ILIKE $2)
  AND ($3 = '' OR type = $3)
  AND ($4::uuid IS NULL OR vat_rate_id = $4)
ORDER BY created_at DESC
LIMIT $5 OFFSET $6;

-- name: CountProducts :one
SELECT COUNT(*) FROM RAC_catalog_products
WHERE organization_id = $1
  AND ($2 = '' OR title ILIKE $2 OR reference ILIKE $2)
  AND ($3 = '' OR type = $3)
  AND ($4::uuid IS NULL OR vat_rate_id = $4);

-- name: UpdateProduct :one
UPDATE RAC_catalog_products
SET
  vat_rate_id = COALESCE($3, vat_rate_id),
  title = COALESCE($4, title),
  reference = COALESCE($5, reference),
  description = COALESCE($6, description),
  price_cents = COALESCE($7, price_cents),
  type = COALESCE($8, type),
  period_count = COALESCE($9, period_count),
  period_unit = COALESCE($10, period_unit),
  updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING id, organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit, created_at, updated_at;

-- name: DeleteProduct :exec
DELETE FROM RAC_catalog_products
WHERE id = $1 AND organization_id = $2;

-- name: GetProductsByIDs :many
SELECT id, organization_id, vat_rate_id, title, reference, description, price_cents, type, period_count, period_unit, created_at, updated_at FROM RAC_catalog_products
WHERE organization_id = $1 AND id = ANY($2::uuid[]);

-- Materials

-- name: AddProductMaterials :exec
INSERT INTO RAC_catalog_product_materials (organization_id, product_id, material_id)
SELECT $1, $2, material_id
FROM RAC_catalog_products p
CROSS JOIN LATERAL unnest($3::uuid[]) AS material_id
WHERE p.id = $2 AND p.organization_id = $1
ON CONFLICT DO NOTHING;

-- name: RemoveProductMaterials :exec
DELETE FROM RAC_catalog_product_materials
WHERE organization_id = $1 AND product_id = $2 AND material_id = ANY($3::uuid[]);

-- name: ListProductMaterials :many
SELECT p.id, p.organization_id, p.vat_rate_id, p.title, p.reference, p.description, p.price_cents, p.type, p.period_count, p.period_unit, p.created_at, p.updated_at FROM RAC_catalog_products p
JOIN RAC_catalog_product_materials pm
  ON pm.material_id = p.id AND pm.organization_id = p.organization_id
WHERE pm.organization_id = $1 AND pm.product_id = $2
ORDER BY p.title ASC;

-- name: HasProductMaterials :one
SELECT EXISTS(SELECT 1 FROM RAC_catalog_product_materials WHERE organization_id = $1 AND product_id = $2);
