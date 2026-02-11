-- +goose Up
-- Google Ads tracking fields and export infrastructure

ALTER TABLE RAC_leads
  ADD COLUMN IF NOT EXISTS gclid TEXT,
  ADD COLUMN IF NOT EXISTS utm_source TEXT,
  ADD COLUMN IF NOT EXISTS utm_medium TEXT,
  ADD COLUMN IF NOT EXISTS utm_campaign TEXT,
  ADD COLUMN IF NOT EXISTS utm_content TEXT,
  ADD COLUMN IF NOT EXISTS utm_term TEXT,
  ADD COLUMN IF NOT EXISTS ad_landing_page TEXT,
  ADD COLUMN IF NOT EXISTS referrer_url TEXT;

CREATE INDEX IF NOT EXISTS idx_leads_gclid ON RAC_leads(gclid) WHERE gclid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_leads_org_created_at ON RAC_leads(organization_id, created_at DESC);

-- API keys for Google Ads exports
CREATE TABLE IF NOT EXISTS RAC_export_api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  key_hash TEXT NOT NULL UNIQUE,
  key_prefix TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_by UUID REFERENCES RAC_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_export_keys_org ON RAC_export_api_keys(organization_id);
CREATE INDEX IF NOT EXISTS idx_export_keys_hash_active ON RAC_export_api_keys(key_hash) WHERE is_active = true;

-- Lead service events for exportable conversion milestones
CREATE TABLE IF NOT EXISTS RAC_lead_service_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
  lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('status_changed', 'pipeline_stage_changed', 'service_created')),
  status TEXT,
  pipeline_stage TEXT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_service_events_org_time ON RAC_lead_service_events(organization_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_lead_service_events_service ON RAC_lead_service_events(lead_service_id, occurred_at DESC);

-- Track exported conversions to avoid duplicates
CREATE TABLE IF NOT EXISTS RAC_google_ads_exports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
  lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
  conversion_name TEXT NOT NULL,
  conversion_time TIMESTAMPTZ NOT NULL,
  conversion_value NUMERIC(12,2),
  gclid TEXT NOT NULL,
  order_id TEXT NOT NULL,
  exported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, order_id, conversion_name)
);

CREATE INDEX IF NOT EXISTS idx_gads_exports_org_time ON RAC_google_ads_exports(organization_id, exported_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_google_ads_exports;
DROP TABLE IF EXISTS RAC_lead_service_events;
DROP TABLE IF EXISTS RAC_export_api_keys;
DROP INDEX IF EXISTS idx_leads_org_created_at;
DROP INDEX IF EXISTS idx_leads_gclid;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS referrer_url;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS ad_landing_page;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS utm_term;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS utm_content;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS utm_campaign;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS utm_medium;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS utm_source;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS gclid;
