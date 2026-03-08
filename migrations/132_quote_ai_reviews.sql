-- +goose Up
CREATE TABLE RAC_quote_ai_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    decision TEXT NOT NULL CHECK (decision IN ('approved', 'needs_repair', 'requires_human')),
    summary TEXT NOT NULL,
    findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    signals JSONB NOT NULL DEFAULT '[]'::jsonb,
    attempt_count INTEGER NOT NULL DEFAULT 1,
    run_id TEXT,
    reviewer_name TEXT,
    model_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quote_ai_reviews_quote_created_at
    ON RAC_quote_ai_reviews (quote_id, created_at DESC);

CREATE INDEX idx_quote_ai_reviews_org_quote_created_at
    ON RAC_quote_ai_reviews (organization_id, quote_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_quote_ai_reviews;