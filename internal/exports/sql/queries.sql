-- name: UpsertGoogleAdsExportCredential :one
INSERT INTO RAC_google_ads_export_credentials (
    organization_id, username, password_hash, password_encrypted, created_by
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (organization_id)
DO UPDATE SET
    username = EXCLUDED.username,
    password_hash = EXCLUDED.password_hash,
    password_encrypted = EXCLUDED.password_encrypted,
    created_by = EXCLUDED.created_by,
    updated_at = now(),
    last_used_at = NULL
RETURNING id, organization_id, username, password_hash, password_encrypted, created_by, created_at, updated_at, last_used_at;

-- name: GetGoogleAdsExportCredentialByUsername :one
SELECT id, organization_id, username, password_hash, password_encrypted, created_by, created_at, updated_at, last_used_at
FROM RAC_google_ads_export_credentials
WHERE username = $1;

-- name: GetGoogleAdsExportCredentialByOrganization :one
SELECT id, organization_id, username, password_hash, password_encrypted, created_by, created_at, updated_at, last_used_at
FROM RAC_google_ads_export_credentials
WHERE organization_id = $1;

-- name: DeleteGoogleAdsExportCredential :execrows
DELETE FROM RAC_google_ads_export_credentials
WHERE organization_id = $1;

-- name: TouchGoogleAdsExportCredential :exec
UPDATE RAC_google_ads_export_credentials
SET last_used_at = now(), updated_at = now()
WHERE id = $1;

-- name: ListGoogleAdsConversionEvents :many
SELECT 
    e.id, e.organization_id, e.lead_id, e.lead_service_id, e.event_type, 
    e.status, e.pipeline_stage, e.occurred_at,
    COALESCE(l.gclid, '') AS gclid,
    l.consumer_email,
    COALESCE(l.consumer_phone, '') AS consumer_phone,
    COALESCE(l.consumer_first_name, '') AS consumer_first_name,
    COALESCE(l.consumer_last_name, '') AS consumer_last_name,
    COALESCE(l.address_street, '') AS address_street,
    COALESCE(l.address_house_number, '') AS address_house_number,
    COALESCE(l.address_city, '') AS address_city,
    COALESCE(l.address_zip_code, '') AS address_zip_code,
    l.projected_value_cents
FROM RAC_lead_service_events e
INNER JOIN RAC_leads l ON l.id = e.lead_id AND l.organization_id = e.organization_id
WHERE e.organization_id = $1
    AND l.deleted_at IS NULL
    AND e.occurred_at >= $2
    AND e.occurred_at <= $3
ORDER BY e.occurred_at ASC
LIMIT $4;