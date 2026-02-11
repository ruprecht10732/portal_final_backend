-- +goose Up
-- Track actions performed on RAC_leads for auditing purposes

CREATE TABLE IF NOT EXISTS RAC_lead_activity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_activity_lead_id ON RAC_lead_activity(lead_id);

-- +goose Down
DROP TABLE IF EXISTS RAC_lead_activity;
