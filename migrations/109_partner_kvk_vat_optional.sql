-- +goose Up
-- Purpose: Make partner KVK/VAT optional (nullable)

ALTER TABLE RAC_partners
  ALTER COLUMN kvk_number DROP NOT NULL,
  ALTER COLUMN vat_number DROP NOT NULL;

-- +goose Down
-- Revert partner KVK/VAT to required (NOT NULL)

ALTER TABLE RAC_partners
  ALTER COLUMN kvk_number SET NOT NULL,
  ALTER COLUMN vat_number SET NOT NULL;
