-- +goose Up
ALTER TABLE RAC_catalog_products
  ADD COLUMN labor_time_text text;

-- +goose Down
ALTER TABLE RAC_catalog_products DROP COLUMN IF EXISTS labor_time_text;
