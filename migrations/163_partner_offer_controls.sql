-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS offer_margin_basis_points INTEGER NOT NULL DEFAULT 1000;

ALTER TABLE RAC_partner_offers
  ADD COLUMN IF NOT EXISTS margin_basis_points INTEGER NOT NULL DEFAULT 1000,
  ADD COLUMN IF NOT EXISTS offer_line_items JSONB;

UPDATE RAC_partner_offers
SET offer_line_items = '[]'::jsonb
WHERE offer_line_items IS NULL;

-- +goose Down
ALTER TABLE RAC_partner_offers
  DROP COLUMN IF EXISTS offer_line_items,
  DROP COLUMN IF EXISTS margin_basis_points;

ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS offer_margin_basis_points;