-- +goose Up
-- Replace legacy export API keys with Google Ads HTTPS Basic Auth credentials

CREATE TABLE IF NOT EXISTS RAC_google_ads_export_credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL UNIQUE REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_by UUID REFERENCES RAC_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_gads_export_credentials_org ON RAC_google_ads_export_credentials(organization_id);
CREATE INDEX IF NOT EXISTS idx_gads_export_credentials_username ON RAC_google_ads_export_credentials(username);

DROP TABLE IF EXISTS RAC_export_api_keys;

-- +goose Down
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

DROP TABLE IF EXISTS RAC_google_ads_export_credentials;
