-- +goose Up
ALTER TABLE RAC_provider_integrations
  ADD COLUMN IF NOT EXISTS access_token TEXT,
  ADD COLUMN IF NOT EXISTS refresh_token TEXT,
  ADD COLUMN IF NOT EXISTS token_expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_id TEXT,
  ADD COLUMN IF NOT EXISTS connected_by UUID REFERENCES RAC_users(id),
  ADD COLUMN IF NOT EXISTS disconnected_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE RAC_provider_integrations
  DROP COLUMN IF EXISTS disconnected_at,
  DROP COLUMN IF EXISTS connected_by,
  DROP COLUMN IF EXISTS administration_id,
  DROP COLUMN IF EXISTS token_expires_at,
  DROP COLUMN IF EXISTS refresh_token,
  DROP COLUMN IF EXISTS access_token;