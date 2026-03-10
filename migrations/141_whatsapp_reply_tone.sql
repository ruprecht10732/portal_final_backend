-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_tone_of_voice TEXT NOT NULL DEFAULT 'warm, practical, and professional';

-- +goose Down
ALTER TABLE RAC_organization_settings
DROP COLUMN IF EXISTS whatsapp_tone_of_voice;