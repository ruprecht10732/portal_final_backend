-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS smtp_host       TEXT,
  ADD COLUMN IF NOT EXISTS smtp_port       INT,
  ADD COLUMN IF NOT EXISTS smtp_username   TEXT,
  ADD COLUMN IF NOT EXISTS smtp_password   TEXT,       -- AES-256-GCM encrypted
  ADD COLUMN IF NOT EXISTS smtp_from_email TEXT,
  ADD COLUMN IF NOT EXISTS smtp_from_name  TEXT;

-- +goose Down
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_from_name;
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_from_email;
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_password;
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_username;
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_port;
ALTER TABLE RAC_organization_settings DROP COLUMN IF EXISTS smtp_host;
