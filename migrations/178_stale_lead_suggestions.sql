-- +goose Up
CREATE TABLE RAC_stale_lead_suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    stale_reason TEXT NOT NULL,
    recommended_action TEXT NOT NULL,
    suggested_contact_message TEXT NOT NULL,
    preferred_contact_channel TEXT NOT NULL DEFAULT 'whatsapp',
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only keep the latest suggestion per service per org.
CREATE UNIQUE INDEX uq_stale_lead_suggestions_service
    ON RAC_stale_lead_suggestions (lead_service_id, organization_id);

CREATE INDEX idx_stale_lead_suggestions_org
    ON RAC_stale_lead_suggestions (organization_id);

-- +goose Down
DROP TABLE IF EXISTS RAC_stale_lead_suggestions;
