-- +goose Up
ALTER TABLE RAC_leads
    ADD COLUMN IF NOT EXISTS whatsapp_opted_in BOOLEAN NOT NULL DEFAULT true;