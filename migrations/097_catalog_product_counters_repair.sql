-- +goose Up
-- 097_catalog_product_counters_repair.sql
-- Repair migration for environments where version history drifted and 096 was marked applied without table creation.

CREATE TABLE IF NOT EXISTS RAC_catalog_product_counters (
    organization_id UUID PRIMARY KEY REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    last_number INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS RAC_catalog_product_counters;
