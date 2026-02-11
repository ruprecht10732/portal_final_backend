-- +goose Up
-- Webhook API keys for external form capture
CREATE TABLE IF NOT EXISTS RAC_webhook_api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL DEFAULT '',
    key_hash        TEXT NOT NULL,
    key_prefix      TEXT NOT NULL,
    allowed_domains TEXT[] NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_api_keys_org ON RAC_webhook_api_keys(organization_id);
CREATE INDEX idx_webhook_api_keys_prefix ON RAC_webhook_api_keys(key_prefix);

-- Add webhook-specific columns to RAC_leads
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS raw_form_data JSONB;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS webhook_source_domain TEXT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS is_incomplete BOOLEAN NOT NULL DEFAULT false;
