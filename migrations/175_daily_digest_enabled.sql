-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN daily_digest_enabled BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE RAC_organization_settings
DROP COLUMN IF EXISTS daily_digest_enabled;
