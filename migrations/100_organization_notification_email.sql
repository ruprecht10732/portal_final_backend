-- +goose Up
ALTER TABLE RAC_organization_settings
    ADD COLUMN IF NOT EXISTS notification_email TEXT;

-- +goose Down
ALTER TABLE RAC_organization_settings
    DROP COLUMN IF EXISTS notification_email;