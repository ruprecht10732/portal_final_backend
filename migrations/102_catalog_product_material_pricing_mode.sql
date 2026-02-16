-- +goose Up
ALTER TABLE RAC_catalog_product_materials
ADD COLUMN IF NOT EXISTS pricing_mode TEXT NOT NULL DEFAULT 'additional';

ALTER TABLE RAC_catalog_product_materials
DROP CONSTRAINT IF EXISTS chk_catalog_product_materials_pricing_mode;

ALTER TABLE RAC_catalog_product_materials
ADD CONSTRAINT chk_catalog_product_materials_pricing_mode
CHECK (pricing_mode IN ('included', 'additional', 'optional'));

-- +goose Down
ALTER TABLE RAC_catalog_product_materials
DROP CONSTRAINT IF EXISTS chk_catalog_product_materials_pricing_mode;

ALTER TABLE RAC_catalog_product_materials
DROP COLUMN IF EXISTS pricing_mode;
