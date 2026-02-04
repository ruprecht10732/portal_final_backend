-- Track visit history for audit trail
-- Each row represents a scheduled visit attempt

CREATE TABLE IF NOT EXISTS visit_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    scheduled_date TIMESTAMPTZ NOT NULL,
    scout_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('completed', 'no_show', 'rescheduled', 'cancelled')),
    measurements TEXT,
    access_difficulty TEXT CHECK (access_difficulty IN ('Low', 'Medium', 'High')),
    notes TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_visit_history_lead_id ON visit_history(lead_id);
CREATE INDEX IF NOT EXISTS idx_visit_history_scheduled_date ON visit_history(scheduled_date);
