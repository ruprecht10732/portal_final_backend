-- +goose Up
-- 096_catalog_product_counters.sql
-- Per-organization counter table for catalog product reference generation.

CREATE TABLE IF NOT EXISTS RAC_catalog_product_counters (
    organization_id UUID PRIMARY KEY REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    last_number INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS RAC_catalog_product_counters;
