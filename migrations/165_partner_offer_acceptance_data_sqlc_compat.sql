-- +goose Up
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS requires_inspection BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS signer_name TEXT;
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS signer_business_name TEXT;
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS signer_address TEXT;
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS signature_data TEXT;
ALTER TABLE RAC_partner_offers ADD COLUMN IF NOT EXISTS pdf_file_key TEXT;

-- +goose Down
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS pdf_file_key;
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS signature_data;
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS signer_address;
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS signer_business_name;
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS signer_name;
ALTER TABLE RAC_partner_offers DROP COLUMN IF EXISTS requires_inspection;