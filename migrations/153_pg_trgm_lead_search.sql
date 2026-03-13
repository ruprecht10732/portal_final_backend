-- +goose Up

-- Enable the pg_trgm extension for fuzzy/similarity text search.
-- This is safe to run repeatedly; IF NOT EXISTS prevents errors on re-runs.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- GIN trigram indexes on the name and phone columns used by lead search.
-- These index types allow both ILIKE pattern matching and similarity scoring,
-- replacing the need for a full sequential scan on large tenants.
CREATE INDEX IF NOT EXISTS idx_rac_leads_first_name_trgm
    ON RAC_leads USING GIN (consumer_first_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_rac_leads_last_name_trgm
    ON RAC_leads USING GIN (consumer_last_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_rac_leads_phone_trgm
    ON RAC_leads USING GIN (consumer_phone gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_rac_leads_email_trgm
    ON RAC_leads USING GIN (consumer_email gin_trgm_ops);

-- +goose Down

DROP INDEX IF EXISTS idx_rac_leads_email_trgm;
DROP INDEX IF EXISTS idx_rac_leads_phone_trgm;
DROP INDEX IF EXISTS idx_rac_leads_last_name_trgm;
DROP INDEX IF EXISTS idx_rac_leads_first_name_trgm;
