-- +goose Up
-- 174: Add page_per_item flag to quotes.
-- When enabled, the PDF renders each line item on a separate page.

ALTER TABLE RAC_quotes
    ADD COLUMN page_per_item BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE RAC_quotes DROP COLUMN IF EXISTS page_per_item;
