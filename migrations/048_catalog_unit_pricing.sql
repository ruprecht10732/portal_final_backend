-- +goose Up
-- Add unit pricing fields to catalog products

ALTER TABLE RAC_catalog_products
  ADD COLUMN unit_price_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN unit_label TEXT;

ALTER TABLE RAC_catalog_products
  ADD CONSTRAINT catalog_products_unit_label_check
  CHECK (
    unit_price_cents = 0
    OR (unit_label IS NOT NULL AND btrim(unit_label) <> '')
  );

-- +goose Down
ALTER TABLE RAC_catalog_products DROP CONSTRAINT IF EXISTS catalog_products_unit_label_check;
ALTER TABLE RAC_catalog_products DROP COLUMN IF EXISTS unit_label;
ALTER TABLE RAC_catalog_products DROP COLUMN IF EXISTS unit_price_cents;
