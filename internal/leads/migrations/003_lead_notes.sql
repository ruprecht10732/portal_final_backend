-- Leads Domain: Internal notes for leads
-- SOFT REFERENCES: author_id stored as UUID without FK constraint

CREATE TABLE IF NOT EXISTS lead_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    author_id UUID NOT NULL,       -- References users.id (soft reference)
    body TEXT NOT NULL CHECK (char_length(body) >= 1 AND char_length(body) <= 2000),
    type TEXT NOT NULL DEFAULT 'note' CHECK (type IN ('note', 'call', 'text', 'email', 'system')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_notes_lead_id ON lead_notes(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_notes_created_at ON lead_notes(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lead_notes_author_id ON lead_notes(author_id);
