-- +goose Up
-- 058: Add "onder voorbehoud van financiering" financing disclaimer flag to quotes.
-- When enabled, the quote proposal shows a financing disclaimer the customer must acknowledge.

ALTER TABLE RAC_quotes
    ADD COLUMN financing_disclaimer BOOLEAN NOT NULL DEFAULT false;
