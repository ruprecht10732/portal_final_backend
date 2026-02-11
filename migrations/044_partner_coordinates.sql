-- +goose Up
ALTER TABLE RAC_partners
  ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_partners_coordinates
  ON RAC_partners (latitude, longitude)
  WHERE latitude IS NOT NULL AND longitude IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_partners_coordinates;
ALTER TABLE RAC_partners DROP COLUMN IF EXISTS latitude;
ALTER TABLE RAC_partners DROP COLUMN IF EXISTS longitude;
