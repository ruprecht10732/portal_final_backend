-- Lead statuses: New, Attempted_Contact, Scheduled, Surveyed, Bad_Lead, Needs_Rescheduling
-- Consumer RAC_roles: Owner, Tenant, Landlord
-- Service types: Windows, Insulation, Solar
-- Access difficulty: Low, Medium, High

CREATE TABLE IF NOT EXISTS RAC_leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Consumer information
    consumer_first_name TEXT NOT NULL,
    consumer_last_name TEXT NOT NULL,
    consumer_phone TEXT NOT NULL,
    consumer_email TEXT,
    consumer_role TEXT NOT NULL DEFAULT 'Owner' CHECK (consumer_role IN ('Owner', 'Tenant', 'Landlord')),
    
    -- Address information
    address_street TEXT NOT NULL,
    address_house_number TEXT NOT NULL,
    address_zip_code TEXT NOT NULL,
    address_city TEXT NOT NULL,
    
    -- Lead details
    service_type TEXT NOT NULL CHECK (service_type IN ('Windows', 'Insulation', 'Solar')),
    status TEXT NOT NULL DEFAULT 'New' CHECK (status IN ('New', 'Attempted_Contact', 'Scheduled', 'Surveyed', 'Bad_Lead', 'Needs_Rescheduling')),
    
    -- Assignment
    assigned_agent_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    viewed_by_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    viewed_at TIMESTAMPTZ,
    
    -- Visit / Survey information
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

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_leads_status ON RAC_leads(status);
CREATE INDEX IF NOT EXISTS idx_leads_assigned_agent ON RAC_leads(assigned_agent_id);
CREATE INDEX IF NOT EXISTS idx_leads_scout ON RAC_leads(visit_scout_id);
CREATE INDEX IF NOT EXISTS idx_leads_phone ON RAC_leads(consumer_phone);
CREATE INDEX IF NOT EXISTS idx_leads_created_at ON RAC_leads(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_leads_scheduled_date ON RAC_leads(visit_scheduled_date) WHERE visit_scheduled_date IS NOT NULL;

-- Add agent and scout RAC_roles
INSERT INTO RAC_roles (name)
VALUES ('agent'), ('scout')
ON CONFLICT (name) DO NOTHING;
