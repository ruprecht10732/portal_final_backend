-- +goose Up

ALTER TABLE RAC_whatsapp_agent_messages
    ADD COLUMN external_message_id TEXT,
    ADD COLUMN metadata JSONB;

CREATE INDEX idx_wa_agent_messages_org_external
    ON RAC_whatsapp_agent_messages (organization_id, external_message_id)
    WHERE external_message_id IS NOT NULL;

CREATE INDEX idx_wa_agent_messages_org_phone_role_time
    ON RAC_whatsapp_agent_messages (organization_id, phone_number, role, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_wa_agent_messages_org_phone_role_time;
DROP INDEX IF EXISTS idx_wa_agent_messages_org_external;

ALTER TABLE RAC_whatsapp_agent_messages
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS external_message_id;