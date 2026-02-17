-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_provider_integrations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  is_connected BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_provider_integrations_org
  ON RAC_provider_integrations(organization_id);

CREATE TABLE IF NOT EXISTS RAC_quote_exports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  external_id TEXT NOT NULL,
  external_url TEXT,
  state TEXT NOT NULL DEFAULT 'draft',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (quote_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_quote_exports_org_provider
  ON RAC_quote_exports(organization_id, provider);

-- +goose Down
DROP TABLE IF EXISTS RAC_quote_exports;
DROP TABLE IF EXISTS RAC_provider_integrations;