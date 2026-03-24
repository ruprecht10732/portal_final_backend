-- +goose Up
ALTER TABLE RAC_organization_settings ADD COLUMN IF NOT EXISTS review_url TEXT;

-- +goose Down
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS review_url;
