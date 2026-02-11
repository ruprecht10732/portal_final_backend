-- +goose Up
-- Enforce exclusive pricing mode for catalog products

ALTER TABLE RAC_catalog_products
  ADD CONSTRAINT catalog_products_pricing_mode_check
  CHECK (
    (price_cents > 0 AND unit_price_cents = 0)
    OR (price_cents = 0 AND unit_price_cents > 0)
  );

-- +goose Down
ALTER TABLE RAC_catalog_products DROP CONSTRAINT IF EXISTS catalog_products_pricing_mode_check;
