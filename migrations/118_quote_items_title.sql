-- +goose Up
ALTER TABLE RAC_quote_items ADD COLUMN title TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE RAC_quote_items DROP COLUMN IF EXISTS title;
