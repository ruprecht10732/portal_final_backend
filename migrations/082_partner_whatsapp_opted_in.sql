-- +goose Up
ALTER TABLE RAC_partners
    ADD COLUMN IF NOT EXISTS whatsapp_opted_in BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE RAC_partners DROP COLUMN IF EXISTS whatsapp_opted_in;