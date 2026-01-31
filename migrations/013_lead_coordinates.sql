ALTER TABLE leads
  ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_leads_coordinates
  ON leads (latitude, longitude)
  WHERE latitude IS NOT NULL AND longitude IS NOT NULL;
