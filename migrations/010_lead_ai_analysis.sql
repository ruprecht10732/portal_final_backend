-- +goose Up
CREATE TABLE lead_ai_analysis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    urgency_level TEXT NOT NULL CHECK (urgency_level IN ('High', 'Medium', 'Low')),
    urgency_reason TEXT,
    talking_points JSONB NOT NULL DEFAULT '[]'::jsonb,
    objection_handling JSONB NOT NULL DEFAULT '[]'::jsonb,
    upsell_opportunities JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_lead_ai_analysis_lead_id ON lead_ai_analysis(lead_id);
CREATE INDEX idx_lead_ai_analysis_created_at ON lead_ai_analysis(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS lead_ai_analysis;
