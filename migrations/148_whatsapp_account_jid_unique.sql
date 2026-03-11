-- +goose Up
DROP INDEX IF EXISTS idx_rac_organization_settings_whatsapp_account_jid;

CREATE UNIQUE INDEX IF NOT EXISTS idx_rac_organization_settings_whatsapp_account_jid_unique
ON RAC_organization_settings (whatsapp_account_jid)
WHERE whatsapp_account_jid IS NOT NULL AND btrim(whatsapp_account_jid) <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_rac_organization_settings_whatsapp_account_jid_unique;

CREATE INDEX IF NOT EXISTS idx_rac_organization_settings_whatsapp_account_jid
ON RAC_organization_settings (whatsapp_account_jid)
WHERE whatsapp_account_jid IS NOT NULL;