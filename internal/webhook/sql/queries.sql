-- name: CreateWebhookTimelineEvent :exec
INSERT INTO lead_timeline_events (
	lead_id,
	service_id,
	organization_id,
	actor_type,
	actor_name,
	event_type,
	title,
	summary,
	metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CreateWebhookAPIKey :one
INSERT INTO RAC_webhook_api_keys (organization_id, name, key_hash, key_prefix, allowed_domains)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at;

-- name: GetWebhookAPIKeyByHash :one
SELECT id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
FROM RAC_webhook_api_keys
WHERE key_hash = $1 AND is_active = true;

-- name: ListWebhookAPIKeysByOrganization :many
SELECT id, organization_id, name, key_hash, key_prefix, allowed_domains, is_active, created_at, updated_at
FROM RAC_webhook_api_keys
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: RevokeWebhookAPIKey :execrows
UPDATE RAC_webhook_api_keys
SET is_active = false, updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: UpdateWebhookLeadData :exec
UPDATE RAC_leads
SET raw_form_data = $3, webhook_source_domain = $4, is_incomplete = $5, updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: FindRecentDuplicateLead :one
SELECT id
FROM RAC_leads
WHERE organization_id = $1
	AND created_at >= now() - make_interval(secs => $2)
	AND (CAST($3 AS text) = '' OR consumer_email = CAST($3 AS text))
	AND (CAST($4 AS text) = '' OR consumer_phone = CAST($4 AS text))
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateGoogleWebhookConfig :one
INSERT INTO RAC_google_webhook_configs (organization_id, name, google_key_hash, google_key_prefix, campaign_mappings)
VALUES ($1, $2, $3, $4, '{}'::jsonb)
RETURNING id;

-- name: GetGoogleWebhookConfigByHash :one
SELECT id, organization_id, name, google_key_hash, google_key_prefix, campaign_mappings, is_active, created_at, updated_at
FROM RAC_google_webhook_configs
WHERE google_key_hash = $1 AND is_active = true;

-- name: ListGoogleWebhookConfigs :many
SELECT id, organization_id, name, google_key_hash, google_key_prefix, campaign_mappings, is_active, created_at, updated_at
FROM RAC_google_webhook_configs
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: UpsertGoogleCampaignMapping :exec
UPDATE RAC_google_webhook_configs
SET campaign_mappings = jsonb_set(campaign_mappings, ARRAY[$3::text], to_jsonb($4::text), true),
	updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: DeleteGoogleCampaignMapping :exec
UPDATE RAC_google_webhook_configs
SET campaign_mappings = campaign_mappings - CAST($3 AS text),
	updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: DeleteGoogleWebhookConfig :execrows
DELETE FROM RAC_google_webhook_configs
WHERE id = $1 AND organization_id = $2;

-- name: GoogleLeadIDExists :one
SELECT EXISTS(SELECT 1 FROM RAC_google_lead_ids WHERE lead_id = $1);

-- name: StoreGoogleLeadID :exec
INSERT INTO RAC_google_lead_ids (lead_id, organization_id, lead_uuid, is_test)
VALUES ($1, $2, $3, $4)
ON CONFLICT (lead_id) DO NOTHING;

-- name: UpdateGoogleLeadMetadata :exec
UPDATE RAC_leads
SET google_campaign_id = $2, google_creative_id = $3, google_adgroup_id = $4, google_form_id = $5, updated_at = now()
WHERE id = $1;

-- name: GetOrganizationGTMContainerID :one
SELECT gtm_container_id
FROM RAC_organizations
WHERE id = $1;

-- name: SetOrganizationGTMContainerID :exec
UPDATE RAC_organizations
SET gtm_container_id = $2, updated_at = now()
WHERE id = $1;

-- name: ClearOrganizationGTMContainerID :exec
UPDATE RAC_organizations
SET gtm_container_id = NULL, updated_at = now()
WHERE id = $1;