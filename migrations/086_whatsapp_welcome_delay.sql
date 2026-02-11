-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_welcome_delay_minutes INT NOT NULL DEFAULT 2;

-- Ensure existing rows have a valid value (defensive for older rows / NULLs).
UPDATE RAC_organization_settings
SET whatsapp_welcome_delay_minutes = 2
WHERE whatsapp_welcome_delay_minutes IS NULL;

-- +goose Down
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS whatsapp_welcome_delay_minutes;
