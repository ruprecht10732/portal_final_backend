-- +goose Up
-- Upgrade price_cents from INTEGER (max ~$21M) to BIGINT for high-value catalogs.
-- This is a non-breaking change; existing data remains valid.
ALTER TABLE catalog_products ALTER COLUMN price_cents TYPE BIGINT;

-- +goose Down
-- Note: Reverting may cause data loss if values exceed INTEGER max.
ALTER TABLE catalog_products ALTER COLUMN price_cents TYPE INTEGER;
