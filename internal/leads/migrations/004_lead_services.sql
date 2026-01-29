-- Leads Domain: Multiple services per lead
-- SOFT REFERENCES: visit_scout_id stored as UUID without FK constraint

CREATE TABLE IF NOT EXISTS lead_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    
    -- Service info
    service_type TEXT NOT NULL CHECK (service_type IN ('Windows', 'Insulation', 'Solar')),
    status TEXT NOT NULL DEFAULT 'New' CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling', 'Closed')),
    
    -- Visit / Survey information (per service)
    visit_scheduled_date TIMESTAMPTZ,
    visit_scout_id UUID,           -- References users.id (soft reference)
    visit_measurements TEXT,
    visit_access_difficulty TEXT CHECK (visit_access_difficulty IS NULL OR visit_access_difficulty IN ('Low', 'Medium', 'High')),
    visit_notes TEXT,
    visit_completed_at TIMESTAMPTZ,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_lead_services_lead_id ON lead_services(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_services_status ON lead_services(status);
CREATE INDEX IF NOT EXISTS idx_lead_services_service_type ON lead_services(service_type);
CREATE INDEX IF NOT EXISTS idx_lead_services_created_at ON lead_services(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lead_services_scout_id ON lead_services(visit_scout_id);
