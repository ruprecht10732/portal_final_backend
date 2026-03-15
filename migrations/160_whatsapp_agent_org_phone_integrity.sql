-- +goose Up

CREATE UNIQUE INDEX IF NOT EXISTS idx_wa_agent_users_org_phone
    ON RAC_whatsapp_agent_users (organization_id, phone_number);

ALTER TABLE RAC_whatsapp_agent_messages
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_messages_org_phone_fkey;

ALTER TABLE RAC_whatsapp_agent_messages
    ADD CONSTRAINT rac_whatsapp_agent_messages_org_phone_fkey
    FOREIGN KEY (organization_id, phone_number)
    REFERENCES RAC_whatsapp_agent_users (organization_id, phone_number)
    ON DELETE CASCADE
    NOT VALID;

-- +goose Down

ALTER TABLE RAC_whatsapp_agent_messages
    DROP CONSTRAINT IF EXISTS rac_whatsapp_agent_messages_org_phone_fkey;

DROP INDEX IF EXISTS idx_wa_agent_users_org_phone;