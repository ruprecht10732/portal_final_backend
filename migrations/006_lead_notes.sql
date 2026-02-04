-- Lead notes for internal comments

CREATE TABLE IF NOT EXISTS RAC_lead_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (char_length(body) >= 1 AND char_length(body) <= 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_notes_lead_id ON RAC_lead_notes(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_notes_created_at ON RAC_lead_notes(created_at DESC);
