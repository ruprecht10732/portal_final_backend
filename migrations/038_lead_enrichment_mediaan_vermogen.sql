-- +goose Up
-- Add mediaan vermogen (median wealth) field from CBS OData API
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_mediaan_vermogen_x1000 DOUBLE PRECISION;

COMMENT ON COLUMN RAC_leads.lead_enrichment_mediaan_vermogen_x1000 IS 'Mediaan vermogen van particuliere huishoudens (× 1000 EUR) - from CBS OData API by buurtcode';

-- +goose Down
SELECT 1;
