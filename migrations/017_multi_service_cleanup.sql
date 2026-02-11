-- +goose Up
-- Migration: Clean up legacy single-service fields from RAC_leads table
-- Services are now exclusively managed via RAC_lead_services table

-- 1. Add consumer_note to RAC_lead_services for per-service intake notes
ALTER TABLE RAC_lead_services ADD COLUMN IF NOT EXISTS consumer_note TEXT;

-- 2. Migrate existing consumer_note from RAC_leads to their first service
UPDATE RAC_lead_services ls
SET consumer_note = l.consumer_note
FROM RAC_leads l
WHERE ls.lead_id = l.id
  AND l.consumer_note IS NOT NULL
  AND ls.consumer_note IS NULL
  AND ls.created_at = (
    SELECT MIN(created_at) FROM RAC_lead_services WHERE lead_id = l.id
  );

-- 3. Remove legacy columns from RAC_leads table
-- These are now managed per-service in RAC_lead_services:
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS service_type;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS status;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS consumer_note;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_scheduled_date;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_scout_id;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_measurements;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_access_difficulty;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_notes;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS visit_completed_at;

-- 4. Add index for returning customer lookup by email
CREATE INDEX IF NOT EXISTS idx_leads_email ON RAC_leads(consumer_email) WHERE consumer_email IS NOT NULL;

-- 5. Add composite index for phone+email lookup
CREATE INDEX IF NOT EXISTS idx_leads_phone_email ON RAC_leads(consumer_phone, consumer_email);

-- +goose Down
ALTER TABLE RAC_leads
  ADD COLUMN IF NOT EXISTS service_type TEXT,
  ADD COLUMN IF NOT EXISTS status TEXT,
  ADD COLUMN IF NOT EXISTS consumer_note TEXT,
  ADD COLUMN IF NOT EXISTS visit_scheduled_date TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS visit_scout_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS visit_measurements TEXT,
  ADD COLUMN IF NOT EXISTS visit_access_difficulty TEXT,
  ADD COLUMN IF NOT EXISTS visit_notes TEXT,
  ADD COLUMN IF NOT EXISTS visit_completed_at TIMESTAMPTZ;

UPDATE RAC_leads l
SET
  service_type = COALESCE(l.service_type, st.name),
  status = COALESCE(l.status, ls.status),
  consumer_note = COALESCE(l.consumer_note, ls.consumer_note),
  visit_scheduled_date = COALESCE(l.visit_scheduled_date, ls.visit_scheduled_date),
  visit_scout_id = COALESCE(l.visit_scout_id, ls.visit_scout_id),
  visit_measurements = COALESCE(l.visit_measurements, ls.visit_measurements),
  visit_access_difficulty = COALESCE(l.visit_access_difficulty, ls.visit_access_difficulty),
  visit_notes = COALESCE(l.visit_notes, ls.visit_notes),
  visit_completed_at = COALESCE(l.visit_completed_at, ls.visit_completed_at)
FROM RAC_lead_services ls
LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id
WHERE ls.lead_id = l.id
  AND ls.created_at = (
    SELECT MIN(created_at) FROM RAC_lead_services WHERE lead_id = l.id
  );

UPDATE RAC_leads SET service_type = 'Windows' WHERE service_type IS NULL;
UPDATE RAC_leads
SET service_type = 'Windows'
WHERE service_type NOT IN ('Windows', 'Insulation', 'Solar');
UPDATE RAC_leads SET status = 'New' WHERE status IS NULL;

ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_service_type_check;
ALTER TABLE RAC_leads ADD CONSTRAINT leads_service_type_check CHECK (service_type IN ('Windows', 'Insulation', 'Solar'));
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE RAC_leads ADD CONSTRAINT leads_status_check CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling', 'Closed'));
ALTER TABLE RAC_leads ALTER COLUMN service_type SET NOT NULL;
ALTER TABLE RAC_leads ALTER COLUMN status SET NOT NULL;

DROP INDEX IF EXISTS idx_leads_phone_email;
DROP INDEX IF EXISTS idx_leads_email;

ALTER TABLE RAC_lead_services DROP COLUMN IF EXISTS consumer_note;
