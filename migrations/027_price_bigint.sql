-- Upgrade price_cents from INTEGER (max ~$21M) to BIGINT for high-value catalogs.
-- This is a non-breaking change; existing data remains valid.
ALTER TABLE RAC_catalog_products ALTER COLUMN price_cents TYPE BIGINT;

