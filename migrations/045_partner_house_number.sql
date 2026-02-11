-- +goose Up
ALTER TABLE RAC_partners
  ADD COLUMN IF NOT EXISTS house_number TEXT;

CREATE INDEX IF NOT EXISTS idx_partners_house_number
  ON RAC_partners (house_number);
