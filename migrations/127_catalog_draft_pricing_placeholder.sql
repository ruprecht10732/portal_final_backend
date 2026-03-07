-- +goose Up
-- Allow draft catalog products to exist without finalized pricing.

ALTER TABLE RAC_catalog_products
  DROP CONSTRAINT IF EXISTS catalog_products_pricing_mode_check;

ALTER TABLE RAC_catalog_products
  ADD CONSTRAINT catalog_products_pricing_mode_check
  CHECK (
    is_draft
    OR (
      (price_cents > 0 AND unit_price_cents = 0)
      OR (price_cents = 0 AND unit_price_cents > 0)
    )
  );

-- +goose Down
ALTER TABLE RAC_catalog_products
  DROP CONSTRAINT IF EXISTS catalog_products_pricing_mode_check;

ALTER TABLE RAC_catalog_products
  ADD CONSTRAINT catalog_products_pricing_mode_check
  CHECK (
    (price_cents > 0 AND unit_price_cents = 0)
    OR (price_cents = 0 AND unit_price_cents > 0)
  );