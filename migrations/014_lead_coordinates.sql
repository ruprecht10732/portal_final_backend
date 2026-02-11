-- +goose Up
ALTER TABLE RAC_leads
  ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_leads_coordinates
  ON RAC_leads (latitude, longitude)
  WHERE latitude IS NOT NULL AND longitude IS NOT NULL;
