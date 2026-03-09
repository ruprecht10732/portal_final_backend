-- +goose Up
ALTER TABLE RAC_organization_settings
ADD COLUMN IF NOT EXISTS whatsapp_account_jid TEXT;

CREATE INDEX IF NOT EXISTS idx_rac_organization_settings_whatsapp_account_jid
ON RAC_organization_settings (whatsapp_account_jid)
WHERE whatsapp_account_jid IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_rac_organization_settings_whatsapp_account_jid;

ALTER TABLE RAC_organization_settings
DROP COLUMN IF EXISTS whatsapp_account_jid;