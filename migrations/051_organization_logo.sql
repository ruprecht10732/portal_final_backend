-- +goose Up
-- Add logo columns to RAC_organizations (matches the partner logo pattern)
ALTER TABLE RAC_organizations
  ADD COLUMN IF NOT EXISTS logo_file_key TEXT,
  ADD COLUMN IF NOT EXISTS logo_file_name TEXT,
  ADD COLUMN IF NOT EXISTS logo_content_type TEXT,
  ADD COLUMN IF NOT EXISTS logo_size_bytes BIGINT;
