-- Add PC4-level enrichment fields with richer CBS data
-- PC4 provides gas, electricity, income, WOZ data that PC6 lacks

-- New fields from CBS Postcode4 API
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_postcode4 TEXT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_data_year INTEGER;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_gem_elektriciteitsverbruik DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_woz_waarde DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_gem_inkomen DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_pct_hoog_inkomen DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_pct_laag_inkomen DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS lead_enrichment_stedelijkheid INTEGER;

COMMENT ON COLUMN RAC_leads.lead_enrichment_postcode4 IS 'Numeric postcode used for PC4 enrichment';
COMMENT ON COLUMN RAC_leads.lead_enrichment_data_year IS 'Year of CBS statistics data (e.g. 2022, 2023, 2024)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_gem_elektriciteitsverbruik IS 'Average electricity usage in kWh per year (PC4)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_woz_waarde IS 'Average WOZ property value in thousands of euros (PC4)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_gem_inkomen IS 'Average household income in thousands of euros (PC4)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_pct_hoog_inkomen IS 'Percentage of households with high income (PC4)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_pct_laag_inkomen IS 'Percentage of households with low income (PC4)';
COMMENT ON COLUMN RAC_leads.lead_enrichment_stedelijkheid IS 'Urbanization level 1=very urban to 5=rural (PC4)';
