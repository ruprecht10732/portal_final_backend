-- +goose Up
-- Adds projected value in cents for KPI metrics

ALTER TABLE RAC_leads
ADD COLUMN IF NOT EXISTS projected_value_cents BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS projected_value_cents;
