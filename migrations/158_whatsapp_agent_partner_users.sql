-- +goose Up

ALTER TABLE RAC_whatsapp_agent_users
    ADD COLUMN IF NOT EXISTS user_type TEXT NOT NULL DEFAULT 'admin',
    ADD COLUMN IF NOT EXISTS partner_id UUID REFERENCES RAC_partners(id) ON DELETE CASCADE;

UPDATE RAC_whatsapp_agent_users
SET user_type = 'admin'
WHERE user_type IS NULL OR trim(user_type) = '';

ALTER TABLE RAC_whatsapp_agent_users
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_users_user_type_check;

ALTER TABLE RAC_whatsapp_agent_users
    ADD CONSTRAINT rac_whatsapp_agent_users_user_type_check
    CHECK (user_type IN ('admin', 'partner'));

ALTER TABLE RAC_whatsapp_agent_users
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_users_partner_user_check;

ALTER TABLE RAC_whatsapp_agent_users
    ADD CONSTRAINT rac_whatsapp_agent_users_partner_user_check
    CHECK (
        (user_type = 'admin' AND partner_id IS NULL) OR
        (user_type = 'partner' AND partner_id IS NOT NULL)
    );

CREATE UNIQUE INDEX IF NOT EXISTS idx_wa_agent_users_org_partner
    ON RAC_whatsapp_agent_users (organization_id, partner_id)
    WHERE partner_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wa_agent_users_partner
    ON RAC_whatsapp_agent_users (partner_id)
    WHERE partner_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_wa_agent_users_partner;
DROP INDEX IF EXISTS idx_wa_agent_users_org_partner;

ALTER TABLE RAC_whatsapp_agent_users
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_users_partner_user_check;

ALTER TABLE RAC_whatsapp_agent_users
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_users_user_type_check;

ALTER TABLE RAC_whatsapp_agent_users
    DROP COLUMN IF EXISTS partner_id,
    DROP COLUMN IF EXISTS user_type;