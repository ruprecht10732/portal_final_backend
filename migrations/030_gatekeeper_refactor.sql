-- +goose Up

-- 1. Service types: intake guidelines for tenant-defined requirements
ALTER TABLE RAC_service_types
ADD COLUMN IF NOT EXISTS intake_guidelines TEXT;

-- 2. Lead AI analysis: remove legacy sales fields
ALTER TABLE RAC_lead_ai_analysis
DROP COLUMN IF EXISTS talking_points,
DROP COLUMN IF EXISTS objection_handling,
DROP COLUMN IF EXISTS upsell_opportunities,
DROP COLUMN IF EXISTS suggested_whatsapp_message;

-- 3. Lead AI analysis: add triage-focused fields
ALTER TABLE RAC_lead_ai_analysis
ADD COLUMN IF NOT EXISTS lead_quality TEXT NOT NULL DEFAULT 'Low'
    CHECK (lead_quality IN ('Junk', 'Low', 'Potential', 'High', 'Urgent')),
ADD COLUMN IF NOT EXISTS recommended_action TEXT NOT NULL DEFAULT 'RequestInfo'
    CHECK (recommended_action IN ('Reject', 'RequestInfo', 'ScheduleSurvey', 'CallImmediately')),
ADD COLUMN IF NOT EXISTS missing_information JSONB NOT NULL DEFAULT '[]'::jsonb,
ADD COLUMN IF NOT EXISTS preferred_contact_channel TEXT NOT NULL DEFAULT 'WhatsApp'
    CHECK (preferred_contact_channel IN ('WhatsApp', 'Email')),
ADD COLUMN IF NOT EXISTS suggested_contact_message TEXT NOT NULL DEFAULT '';

-- 4. Ensure lead_service_id is populated and required
UPDATE RAC_lead_ai_analysis aa
SET lead_service_id = (
    SELECT ls.id FROM RAC_lead_services ls
    WHERE ls.lead_id = aa.lead_id
    ORDER BY ls.created_at DESC
    LIMIT 1
)
WHERE aa.lead_service_id IS NULL;

-- Remove orphaned analyses without a service
DELETE FROM RAC_lead_ai_analysis WHERE lead_service_id IS NULL;

ALTER TABLE RAC_lead_ai_analysis
ALTER COLUMN lead_service_id SET NOT NULL;

