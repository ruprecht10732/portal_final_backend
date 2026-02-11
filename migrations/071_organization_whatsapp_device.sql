-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_device_id TEXT;
