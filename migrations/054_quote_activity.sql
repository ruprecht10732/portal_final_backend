-- +goose Up
-- Track customer interactions with quotes for auditing / activity timeline

CREATE TABLE IF NOT EXISTS RAC_quote_activity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_quote_activity_quote_id ON RAC_quote_activity(quote_id);
CREATE INDEX IF NOT EXISTS idx_quote_activity_org_id ON RAC_quote_activity(organization_id);

-- +goose Down
DROP TABLE IF EXISTS RAC_quote_activity;
