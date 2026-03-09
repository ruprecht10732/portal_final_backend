-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_whatsapp_messages_org_external_message_id
    ON RAC_whatsapp_messages (organization_id, external_message_id)
    WHERE external_message_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_whatsapp_messages_org_external_message_id;