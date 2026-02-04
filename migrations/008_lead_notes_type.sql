-- Add note type to lead notes

ALTER TABLE RAC_lead_notes
  ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'note';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'lead_notes_type_check'
  ) THEN
    ALTER TABLE RAC_lead_notes
      ADD CONSTRAINT lead_notes_type_check
      CHECK (type IN ('note', 'call', 'text', 'email', 'system'));
  END IF;
END $$;
