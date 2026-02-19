-- +goose Up
-- Add draft flag to catalog products so the system can create products that
-- require admin review (e.g., pricing) without polluting AI quoting.
ALTER TABLE RAC_catalog_products
  ADD COLUMN IF NOT EXISTS is_draft BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE RAC_catalog_products
  DROP COLUMN IF EXISTS is_draft;
