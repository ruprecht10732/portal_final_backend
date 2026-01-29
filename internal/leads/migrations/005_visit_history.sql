-- Leads Domain: Visit history for audit trail
-- SOFT REFERENCES: scout_id stored as UUID without FK constraint

CREATE TABLE IF NOT EXISTS visit_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    scheduled_date TIMESTAMPTZ NOT NULL,
    scout_id UUID,                 -- References users.id (soft reference)
    outcome TEXT NOT NULL CHECK (outcome IN ('completed', 'no_show', 'rescheduled', 'cancelled')),
    measurements TEXT,
    access_difficulty TEXT CHECK (access_difficulty IN ('Low', 'Medium', 'High')),
    notes TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_visit_history_lead_id ON visit_history(lead_id);
CREATE INDEX IF NOT EXISTS idx_visit_history_scheduled_date ON visit_history(scheduled_date);
CREATE INDEX IF NOT EXISTS idx_visit_history_scout_id ON visit_history(scout_id);
