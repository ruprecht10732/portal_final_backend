-- Migration: Clean up legacy single-service fields from leads table
-- Services are now exclusively managed via lead_services table

-- 1. Add consumer_note to lead_services for per-service intake notes
ALTER TABLE lead_services ADD COLUMN IF NOT EXISTS consumer_note TEXT;

-- 2. Migrate existing consumer_note from leads to their first service
UPDATE lead_services ls
SET consumer_note = l.consumer_note
FROM leads l
WHERE ls.lead_id = l.id
  AND l.consumer_note IS NOT NULL
  AND ls.consumer_note IS NULL
  AND ls.created_at = (
    SELECT MIN(created_at) FROM lead_services WHERE lead_id = l.id
  );

-- 3. Remove legacy columns from leads table
-- These are now managed per-service in lead_services:
ALTER TABLE leads DROP COLUMN IF EXISTS service_type;
ALTER TABLE leads DROP COLUMN IF EXISTS status;
ALTER TABLE leads DROP COLUMN IF EXISTS consumer_note;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_scheduled_date;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_scout_id;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_measurements;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_access_difficulty;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_notes;
ALTER TABLE leads DROP COLUMN IF EXISTS visit_completed_at;

-- 4. Add index for returning customer lookup by email
CREATE INDEX IF NOT EXISTS idx_leads_email ON leads(consumer_email) WHERE consumer_email IS NOT NULL;

-- 5. Add composite index for phone+email lookup
CREATE INDEX IF NOT EXISTS idx_leads_phone_email ON leads(consumer_phone, consumer_email);
