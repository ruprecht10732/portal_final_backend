-- +goose Up
-- Remove legacy enrichment fields that are not available in PDOK CBS APIs
-- These were from the old CBS OData API which is no longer used

ALTER TABLE RAC_leads DROP COLUMN IF EXISTS lead_enrichment_woningtype_code;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS lead_enrichment_bouwjaarklasse_code;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS lead_enrichment_woningeigendom_code;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS lead_enrichment_inkomen_code;
