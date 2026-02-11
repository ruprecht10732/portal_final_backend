-- +goose Up
-- Add logo columns to RAC_organizations (matches the partner logo pattern)
ALTER TABLE RAC_organizations
  ADD COLUMN IF NOT EXISTS logo_file_key TEXT,
  ADD COLUMN IF NOT EXISTS logo_file_name TEXT,
  ADD COLUMN IF NOT EXISTS logo_content_type TEXT,
  ADD COLUMN IF NOT EXISTS logo_size_bytes BIGINT;

-- +goose Down
ALTER TABLE RAC_organizations
  DROP COLUMN IF EXISTS logo_size_bytes,
  DROP COLUMN IF EXISTS logo_content_type,
  DROP COLUMN IF EXISTS logo_file_name,
  DROP COLUMN IF EXISTS logo_file_key;
