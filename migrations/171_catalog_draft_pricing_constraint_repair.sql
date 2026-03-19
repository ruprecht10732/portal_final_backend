-- +goose Up
-- Repair: migration 127 relaxed catalog_products_pricing_mode_check to allow
-- draft products with price_cents=0 AND unit_price_cents=0. On databases that
-- were bootstrapped before migration tracking existed, migration 127 was
-- incorrectly seeded as already-applied (the bootstrap seeder could not detect
-- constraint-only migrations), so the old strict constraint remained in place.
-- This migration re-applies the fix idempotently.

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
