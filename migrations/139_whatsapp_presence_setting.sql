-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_presence TEXT NOT NULL DEFAULT 'available';

UPDATE RAC_organization_settings
SET whatsapp_presence = 'available'
WHERE TRIM(COALESCE(whatsapp_presence, '')) = '';

-- +goose Down
ALTER TABLE RAC_organization_settings
DROP COLUMN IF EXISTS whatsapp_presence;