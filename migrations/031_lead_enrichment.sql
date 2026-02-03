-- Add lead enrichment and scoring fields to leads table
-- Data is fetched from PDOK/CBS and cached per lead

ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_source TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_postcode6 TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_buurtcode TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_woningtype_code TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_bouwjaarklasse_code INTEGER;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_woningeigendom_code INTEGER;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_inkomen_code INTEGER;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_gem_aardgasverbruik DOUBLE PRECISION;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_huishouden_grootte DOUBLE PRECISION;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_koopwoningen_pct DOUBLE PRECISION;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_bouwjaar_vanaf2000_pct DOUBLE PRECISION;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_mediaan_vermogen_x1000 DOUBLE PRECISION;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_huishoudens_met_kinderen_pct DOUBLE PRECISION;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_confidence DOUBLE PRECISION;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_enrichment_fetched_at TIMESTAMPTZ;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_score INTEGER;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_score_pre_ai INTEGER;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_score_factors JSONB;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_score_version TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS lead_score_updated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_leads_lead_score ON leads(lead_score) WHERE lead_score IS NOT NULL;

COMMENT ON COLUMN leads.lead_enrichment_source IS 'Source for lead enrichment (pc6 or buurt)';
COMMENT ON COLUMN leads.lead_enrichment_postcode6 IS 'Normalized PC6 postcode used for enrichment';
COMMENT ON COLUMN leads.lead_enrichment_buurtcode IS 'CBS buurtcode used for fallback enrichment';
COMMENT ON COLUMN leads.lead_enrichment_woningtype_code IS 'CBS woningtype code (PC6)';
COMMENT ON COLUMN leads.lead_enrichment_bouwjaarklasse_code IS 'CBS bouwjaarklasse code (PC6)';
COMMENT ON COLUMN leads.lead_enrichment_woningeigendom_code IS 'CBS woningeigendom code (PC6)';
COMMENT ON COLUMN leads.lead_enrichment_inkomen_code IS 'CBS inkomen code (PC6 scale 1-10)';
COMMENT ON COLUMN leads.lead_enrichment_gem_aardgasverbruik IS 'CBS gemiddelde aardgasverbruik (PC6)';
COMMENT ON COLUMN leads.lead_enrichment_huishouden_grootte IS 'CBS huishouden grootte (PC6)';
COMMENT ON COLUMN leads.lead_enrichment_koopwoningen_pct IS 'CBS percentage koopwoningen (buurt fallback)';
COMMENT ON COLUMN leads.lead_enrichment_bouwjaar_vanaf2000_pct IS 'CBS percentage woningen bouwjaar vanaf 2000 (buurt fallback)';
COMMENT ON COLUMN leads.lead_enrichment_mediaan_vermogen_x1000 IS 'CBS mediaan vermogen (x1000 euro, buurt fallback)';
COMMENT ON COLUMN leads.lead_enrichment_huishoudens_met_kinderen_pct IS 'CBS percentage huishoudens met kinderen (buurt fallback)';
COMMENT ON COLUMN leads.lead_enrichment_confidence IS 'Confidence multiplier for enrichment quality (0-1)';
COMMENT ON COLUMN leads.lead_enrichment_fetched_at IS 'When we last fetched lead enrichment data';
COMMENT ON COLUMN leads.lead_score IS 'Final lead score (0-100)';
COMMENT ON COLUMN leads.lead_score_pre_ai IS 'Deterministic pre-AI lead score (0-100)';
COMMENT ON COLUMN leads.lead_score_factors IS 'JSON factors used to compute the lead score';
COMMENT ON COLUMN leads.lead_score_version IS 'Scoring model version identifier';
COMMENT ON COLUMN leads.lead_score_updated_at IS 'When we last calculated lead score';
