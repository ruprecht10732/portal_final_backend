-- +goose Up
ALTER TABLE RAC_quotes
    ADD COLUMN IF NOT EXISTS subsidy_payload JSONB;

-- +goose Down
ALTER TABLE RAC_quotes
    DROP COLUMN IF EXISTS subsidy_payload;
