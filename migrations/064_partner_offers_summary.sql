-- +goose Up
-- Add short job summary to partner offers for partner-facing display
ALTER TABLE RAC_partner_offers
  ADD COLUMN IF NOT EXISTS job_summary_short TEXT;
