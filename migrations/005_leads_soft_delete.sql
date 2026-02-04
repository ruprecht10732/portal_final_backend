ALTER TABLE RAC_leads
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_leads_deleted_at ON RAC_leads(deleted_at);
