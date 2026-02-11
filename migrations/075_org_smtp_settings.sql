ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS smtp_host       TEXT,
  ADD COLUMN IF NOT EXISTS smtp_port       INT,
  ADD COLUMN IF NOT EXISTS smtp_username   TEXT,
  ADD COLUMN IF NOT EXISTS smtp_password   TEXT,       -- AES-256-GCM encrypted
  ADD COLUMN IF NOT EXISTS smtp_from_email TEXT,
  ADD COLUMN IF NOT EXISTS smtp_from_name  TEXT;
