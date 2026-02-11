-- +goose Up
-- Add energy label data to RAC_leads table
-- Data is fetched from EP-Online (RVO) API and cached per lead

ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_class TEXT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_index DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_bouwjaar INTEGER;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_gebouwtype TEXT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_label_valid_until TIMESTAMPTZ;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_label_registered_at TIMESTAMPTZ;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_primair_fossiel DOUBLE PRECISION;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_bag_verblijfsobject_id TEXT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS energy_label_fetched_at TIMESTAMPTZ;

-- Index for potential reporting queries
CREATE INDEX IF NOT EXISTS idx_leads_energy_class ON RAC_leads(energy_class) WHERE energy_class IS NOT NULL;

COMMENT ON COLUMN RAC_leads.energy_class IS 'Energy label class from EP-Online (A+++, A++, A+, A, B, C, D, E, F, G)';
COMMENT ON COLUMN RAC_leads.energy_index IS 'Energy index value from EP-Online';
COMMENT ON COLUMN RAC_leads.energy_bouwjaar IS 'Construction year from EP-Online';
COMMENT ON COLUMN RAC_leads.energy_gebouwtype IS 'Building type from EP-Online (e.g., Vrijstaande woning)';
COMMENT ON COLUMN RAC_leads.energy_label_valid_until IS 'Energy label validity end date';
COMMENT ON COLUMN RAC_leads.energy_label_registered_at IS 'When the energy label was registered at RVO';
COMMENT ON COLUMN RAC_leads.energy_primair_fossiel IS 'Primary fossil energy use in kWh/m2·jaar';
COMMENT ON COLUMN RAC_leads.energy_bag_verblijfsobject_id IS 'BAG adresseerbaar object ID for future lookups';
COMMENT ON COLUMN RAC_leads.energy_label_fetched_at IS 'When we last fetched this energy label data';
