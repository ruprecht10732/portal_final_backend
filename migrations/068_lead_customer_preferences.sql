-- +goose Up
ALTER TABLE RAC_lead_services
    ADD COLUMN IF NOT EXISTS customer_preferences JSONB DEFAULT '{}'::jsonb;
