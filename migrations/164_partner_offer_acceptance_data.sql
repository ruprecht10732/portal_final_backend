-- +goose Up
ALTER TABLE RAC_partner_offers
    ADD COLUMN IF NOT EXISTS requires_inspection BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS signer_name         TEXT,
    ADD COLUMN IF NOT EXISTS signer_business_name TEXT,
    ADD COLUMN IF NOT EXISTS signer_address      TEXT,
    ADD COLUMN IF NOT EXISTS signature_data      TEXT,
    ADD COLUMN IF NOT EXISTS pdf_file_key        TEXT;

-- +goose Down
ALTER TABLE RAC_partner_offers
    DROP COLUMN IF EXISTS requires_inspection,
    DROP COLUMN IF EXISTS signer_name,
    DROP COLUMN IF EXISTS signer_business_name,
    DROP COLUMN IF EXISTS signer_address,
    DROP COLUMN IF EXISTS signature_data,
    DROP COLUMN IF EXISTS pdf_file_key;
