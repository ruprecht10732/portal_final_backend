-- +goose Up
-- Migration: AI analysis is now per-service instead of per-lead
-- This allows different services on the same lead to have separate AI advice

-- 1. Add lead_service_id column to RAC_lead_ai_analysis
ALTER TABLE RAC_lead_ai_analysis ADD COLUMN IF NOT EXISTS lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE CASCADE;

-- 2. Migrate existing analyses to the oldest service of each lead
UPDATE RAC_lead_ai_analysis aa
SET lead_service_id = (
    SELECT ls.id FROM RAC_lead_services ls
    WHERE ls.lead_id = aa.lead_id
    ORDER BY ls.created_at ASC
    LIMIT 1
)
WHERE aa.lead_service_id IS NULL;

-- 3. For any analyses without a matching service, create a placeholder or delete them
-- We'll keep them with NULL for now as some RAC_leads might not have services yet
-- The application will handle this gracefully

-- 4. Create index for efficient lookup by service
CREATE INDEX IF NOT EXISTS idx_lead_ai_analysis_service_id ON RAC_lead_ai_analysis(lead_service_id);

-- 5. Keep lead_id for reference but primary lookup will be by service_id
-- Note: We don't make lead_service_id NOT NULL to handle edge cases
