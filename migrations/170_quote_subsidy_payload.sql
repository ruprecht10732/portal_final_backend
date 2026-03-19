-- +goose Up
ALTER TABLE RAC_quotes
    ADD COLUMN IF NOT EXISTS subsidy_payload JSONB;
