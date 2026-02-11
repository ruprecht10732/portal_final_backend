-- +goose Up
-- Add note type to lead notes

ALTER TABLE RAC_lead_notes
  ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'note';

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    -- nosemgrep: system catalog query required for idempotent constraint creation
    SELECT 1 FROM pg_constraint WHERE conname = 'lead_notes_type_check'
  ) THEN
    ALTER TABLE RAC_lead_notes
      ADD CONSTRAINT lead_notes_type_check
      CHECK (type IN ('note', 'call', 'text', 'email', 'system'));
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE RAC_lead_notes DROP CONSTRAINT IF EXISTS lead_notes_type_check;
ALTER TABLE RAC_lead_notes DROP COLUMN IF EXISTS type;
