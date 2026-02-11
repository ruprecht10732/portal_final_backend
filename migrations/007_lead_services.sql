-- +goose Up
-- Lead services table: allows multiple services per lead with per-service status and visit info

CREATE TABLE IF NOT EXISTS RAC_lead_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    
    -- Service info
    service_type TEXT NOT NULL CHECK (service_type IN ('Windows', 'Insulation', 'Solar')),
    status TEXT NOT NULL DEFAULT 'New' CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling', 'Closed')),
    
    -- Visit / Survey information (per service)
    visit_scheduled_date TIMESTAMPTZ,
    visit_scout_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    visit_measurements TEXT,
    visit_access_difficulty TEXT CHECK (visit_access_difficulty IS NULL OR visit_access_difficulty IN ('Low', 'Medium', 'High')),
    visit_notes TEXT,
    visit_completed_at TIMESTAMPTZ,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_lead_services_lead_id ON RAC_lead_services(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_services_status ON RAC_lead_services(status);
CREATE INDEX IF NOT EXISTS idx_lead_services_service_type ON RAC_lead_services(service_type);
CREATE INDEX IF NOT EXISTS idx_lead_services_created_at ON RAC_lead_services(created_at DESC);

-- Migrate existing RAC_leads into RAC_lead_services
INSERT INTO RAC_lead_services (
    lead_id,
    service_type,
    status,
    visit_scheduled_date,
    visit_scout_id,
    visit_measurements,
    visit_access_difficulty,
    visit_notes,
    visit_completed_at,
    created_at,
    updated_at
)
SELECT id, service_type, status, visit_scheduled_date, visit_scout_id, visit_measurements,
    visit_access_difficulty, visit_notes, visit_completed_at, created_at, updated_at
FROM RAC_leads
WHERE deleted_at IS NULL;

-- Add 'Closed' to RAC_leads status constraint (for backward compatibility during transition)
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE RAC_leads ADD CONSTRAINT leads_status_check CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling', 'Closed'));

-- +goose Down
UPDATE RAC_leads SET status = 'New' WHERE status = 'Closed';
ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE RAC_leads ADD CONSTRAINT leads_status_check CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling'));
DROP TABLE IF EXISTS RAC_lead_services;
