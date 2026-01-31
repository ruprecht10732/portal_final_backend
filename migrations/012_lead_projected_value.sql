-- Adds projected value in cents for KPI metrics

ALTER TABLE leads
ADD COLUMN IF NOT EXISTS projected_value_cents BIGINT NOT NULL DEFAULT 0;
