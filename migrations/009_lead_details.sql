-- Add details to capture the initial user request
ALTER TABLE leads 
ADD COLUMN consumer_note TEXT,
ADD COLUMN source TEXT DEFAULT 'manual'; -- e.g., 'website', 'referral', 'manual'
