-- +goose Up
ALTER TABLE RAC_whatsapp_conversations
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_whatsapp_conversations_org_visibility_last_message
    ON RAC_whatsapp_conversations (organization_id, deleted_at, archived_at, last_message_at DESC, updated_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_whatsapp_conversations_org_visibility_last_message;

ALTER TABLE RAC_whatsapp_conversations
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS archived_at;