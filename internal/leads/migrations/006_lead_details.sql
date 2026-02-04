-- Leads Domain: Add details to capture the initial user request

ALTER TABLE RAC_leads
ADD COLUMN IF NOT EXISTS consumer_note TEXT,
ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'manual';
