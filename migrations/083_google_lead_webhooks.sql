-- +goose Up
-- Google Lead Form webhook configurations
CREATE TABLE IF NOT EXISTS RAC_google_webhook_configs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    name              TEXT NOT NULL DEFAULT '',
    google_key_hash   TEXT NOT NULL UNIQUE,
    google_key_prefix TEXT NOT NULL,
    campaign_mappings JSONB NOT NULL DEFAULT '{}',  -- Maps campaign_id to service_type: {"12345": "windows", "67890": "insulation"}
    is_active         BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_google_webhook_configs_org ON RAC_google_webhook_configs(organization_id);
CREATE INDEX idx_google_webhook_configs_key_hash ON RAC_google_webhook_configs(google_key_hash) WHERE is_active = true;

-- Google lead deduplication tracking
-- Google does not guarantee exactly-once delivery, so we track lead_id to prevent duplicates
CREATE TABLE IF NOT EXISTS RAC_google_lead_ids (
    lead_id         TEXT PRIMARY KEY,  -- Google's unique lead identifier
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_uuid       UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,  -- Our internal lead ID (if created)
    is_test         BOOLEAN NOT NULL DEFAULT false,  -- Test leads are logged but don't create production records
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_google_lead_ids_org ON RAC_google_lead_ids(organization_id);
CREATE INDEX idx_google_lead_ids_created ON RAC_google_lead_ids(created_at DESC);

-- Add Google-specific columns to RAC_leads for tracking attribution
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS google_campaign_id BIGINT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS google_adgroup_id BIGINT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS google_creative_id BIGINT;
ALTER TABLE RAC_leads ADD COLUMN IF NOT EXISTS google_form_id BIGINT;
