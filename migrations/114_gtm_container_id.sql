-- +goose Up
-- Store an optional Google Tag Manager container ID per organization for the webhook SDK.
ALTER TABLE RAC_organizations
ADD COLUMN IF NOT EXISTS gtm_container_id VARCHAR(20);

-- +goose Down
ALTER TABLE RAC_organizations
DROP COLUMN IF EXISTS gtm_container_id;
