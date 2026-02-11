-- +goose Up
-- Add details to capture the initial user request
ALTER TABLE RAC_leads 
ADD COLUMN consumer_note TEXT,
ADD COLUMN source TEXT DEFAULT 'manual'; -- e.g., 'website', 'referral', 'manual'

-- +goose Down
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS source;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS consumer_note;
