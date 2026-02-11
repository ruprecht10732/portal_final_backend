-- +goose Up
-- Scope lead notes to individual services.
-- service_id is nullable for backward compatibility with pre-existing notes.

ALTER TABLE RAC_lead_notes
  ADD COLUMN IF NOT EXISTS service_id UUID REFERENCES RAC_lead_services(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_lead_notes_service ON RAC_lead_notes(lead_id, service_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_lead_notes_service;
ALTER TABLE RAC_lead_notes DROP COLUMN IF EXISTS service_id;
