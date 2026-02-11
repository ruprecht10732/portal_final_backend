-- +goose Up
-- Safety migration: if 086 was marked applied without executing (bootstrap bug),
-- this will still ensure the column exists in production.
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_welcome_delay_minutes INT NOT NULL DEFAULT 2;

-- +goose Down
-- No-op: keep the column if it exists.
