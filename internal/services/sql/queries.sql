-- name: GetServiceTypeByID :one
SELECT id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at
FROM RAC_service_types
WHERE id = $1 AND organization_id = $2;

-- name: GetServiceTypeBySlug :one
SELECT id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at
FROM RAC_service_types
WHERE slug = $1 AND organization_id = $2;

-- name: ListServiceTypes :many
SELECT id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at
FROM RAC_service_types
WHERE organization_id = $1
ORDER BY name ASC;

-- name: ListActiveServiceTypes :many
SELECT id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at
FROM RAC_service_types
WHERE organization_id = $1 AND is_active = true
ORDER BY name ASC;

-- name: CountServiceTypes :one
SELECT COUNT(*)::int AS countValue
FROM RAC_service_types
WHERE organization_id = sqlc.arg('organization_id')
  AND (sqlc.narg('search')::text IS NULL OR name ILIKE sqlc.narg('search')::text OR slug ILIKE sqlc.narg('search')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: ListServiceTypesWithFilters :many
SELECT id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at
FROM RAC_service_types
WHERE organization_id = sqlc.arg('organization_id')
  AND (sqlc.narg('search')::text IS NULL OR name ILIKE sqlc.narg('search')::text OR slug ILIKE sqlc.narg('search')::text)
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY
  CASE WHEN sqlc.arg('sort_by')::text = 'name' AND sqlc.arg('sort_order')::text = 'asc' THEN name END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'name' AND sqlc.arg('sort_order')::text = 'desc' THEN name END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'slug' AND sqlc.arg('sort_order')::text = 'asc' THEN slug END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'slug' AND sqlc.arg('sort_order')::text = 'desc' THEN slug END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'isActive' AND sqlc.arg('sort_order')::text = 'asc' THEN is_active END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'isActive' AND sqlc.arg('sort_order')::text = 'desc' THEN is_active END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdAt' AND sqlc.arg('sort_order')::text = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdAt' AND sqlc.arg('sort_order')::text = 'desc' THEN created_at END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'updatedAt' AND sqlc.arg('sort_order')::text = 'asc' THEN updated_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'updatedAt' AND sqlc.arg('sort_order')::text = 'desc' THEN updated_at END DESC,
  name ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ServiceTypeExists :one
SELECT EXISTS(SELECT 1 FROM RAC_service_types WHERE id = $1 AND organization_id = $2);

-- name: ServiceTypeHasLeadServices :one
SELECT EXISTS(SELECT 1 FROM RAC_lead_services WHERE service_type_id = $1 AND organization_id = $2);

-- name: CreateServiceType :one
INSERT INTO RAC_service_types (
  organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at;

-- name: UpdateServiceType :one
UPDATE RAC_service_types
SET name = COALESCE(sqlc.narg('name'), name),
    slug = COALESCE(sqlc.narg('slug'), slug),
    description = COALESCE(sqlc.narg('description'), description),
    intake_guidelines = COALESCE(sqlc.narg('intake_guidelines'), intake_guidelines),
    estimation_guidelines = COALESCE(sqlc.narg('estimation_guidelines'), estimation_guidelines),
    icon = COALESCE(sqlc.narg('icon'), icon),
    color = COALESCE(sqlc.narg('color'), color),
    updated_at = now()
WHERE id = sqlc.arg('id') AND organization_id = sqlc.arg('organization_id')
RETURNING id, organization_id, name, slug, description, intake_guidelines, estimation_guidelines, icon, color, is_active, created_at, updated_at;

-- name: DeleteServiceType :execrows
DELETE FROM RAC_service_types
WHERE id = $1 AND organization_id = $2;

-- name: SetServiceTypeActive :execrows
UPDATE RAC_service_types
SET is_active = $3, updated_at = now()
WHERE id = $1 AND organization_id = $2;
