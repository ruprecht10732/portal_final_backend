-- Add source to lead_services for per-service tracking of lead source
ALTER TABLE lead_services ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'manual';

-- Migrate source from leads to the oldest service per lead (if not already set from previous migration)
UPDATE lead_services ls
SET source = COALESCE(
    (SELECT l.source FROM leads l WHERE l.id = ls.lead_id),
    'manual'
)
WHERE ls.source IS NULL OR ls.source = 'manual';
