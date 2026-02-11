-- +goose Up
ALTER TABLE RAC_quotes
  ADD COLUMN IF NOT EXISTS created_by_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_quotes_created_by ON RAC_quotes(created_by_id);
