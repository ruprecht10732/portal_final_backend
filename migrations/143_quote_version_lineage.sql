-- +goose Up

ALTER TABLE RAC_quotes
    ADD COLUMN IF NOT EXISTS duplicated_from_quote_id UUID REFERENCES RAC_quotes(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS previous_version_quote_id UUID REFERENCES RAC_quotes(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS version_root_quote_id UUID REFERENCES RAC_quotes(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS version_number INTEGER NOT NULL DEFAULT 1;

ALTER TABLE RAC_quotes
    DROP CONSTRAINT IF EXISTS rac_quotes_version_number_positive;

ALTER TABLE RAC_quotes
    ADD CONSTRAINT rac_quotes_version_number_positive CHECK (version_number >= 1);

CREATE INDEX IF NOT EXISTS idx_quotes_duplicated_from_quote_id
    ON RAC_quotes (duplicated_from_quote_id);

CREATE INDEX IF NOT EXISTS idx_quotes_previous_version_quote_id
    ON RAC_quotes (previous_version_quote_id);

CREATE INDEX IF NOT EXISTS idx_quotes_version_root_quote_id
    ON RAC_quotes (version_root_quote_id);

-- +goose Down

DROP INDEX IF EXISTS idx_quotes_version_root_quote_id;
DROP INDEX IF EXISTS idx_quotes_previous_version_quote_id;
DROP INDEX IF EXISTS idx_quotes_duplicated_from_quote_id;

ALTER TABLE RAC_quotes
    DROP CONSTRAINT IF EXISTS rac_quotes_version_number_positive;

ALTER TABLE RAC_quotes
    DROP COLUMN IF EXISTS version_number,
    DROP COLUMN IF EXISTS version_root_quote_id,
    DROP COLUMN IF EXISTS previous_version_quote_id,
    DROP COLUMN IF EXISTS duplicated_from_quote_id;