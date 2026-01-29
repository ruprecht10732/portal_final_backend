-- Leads Domain: Activity tracking for leads
-- SOFT REFERENCES: user_id stored as UUID without FK constraint

CREATE TABLE IF NOT EXISTS lead_activity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,         -- References users.id (soft reference)
    action TEXT NOT NULL,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_activity_lead_id ON lead_activity(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_activity_user_id ON lead_activity(user_id);
