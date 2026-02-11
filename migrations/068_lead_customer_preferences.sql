-- +goose Up
ALTER TABLE RAC_lead_services
    ADD COLUMN IF NOT EXISTS customer_preferences JSONB DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE RAC_lead_services DROP COLUMN IF EXISTS customer_preferences;
