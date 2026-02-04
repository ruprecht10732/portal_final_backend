-- Add source to RAC_lead_services for per-service tracking of lead source
ALTER TABLE RAC_lead_services ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'manual';

-- Migrate source from RAC_leads to the oldest service per lead (if not already set from previous migration)
UPDATE RAC_lead_services ls
SET source = COALESCE(
    (SELECT l.source FROM RAC_leads l WHERE l.id = ls.lead_id),
    'manual'
)
WHERE ls.source IS NULL OR ls.source = 'manual';
