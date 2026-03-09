-- +goose Up
ALTER TABLE RAC_whatsapp_conversations
    DROP CONSTRAINT IF EXISTS rac_whatsapp_conversations_last_message_status_check;

ALTER TABLE RAC_whatsapp_conversations
    ADD CONSTRAINT rac_whatsapp_conversations_last_message_status_check
    CHECK (last_message_status IN ('received', 'sent', 'delivered', 'read', 'failed'));

ALTER TABLE RAC_whatsapp_messages
    DROP CONSTRAINT IF EXISTS rac_whatsapp_messages_status_check;

ALTER TABLE RAC_whatsapp_messages
    ADD CONSTRAINT rac_whatsapp_messages_status_check
    CHECK (status IN ('received', 'sent', 'delivered', 'read', 'failed'));

-- +goose Down
ALTER TABLE RAC_whatsapp_messages
    DROP CONSTRAINT IF EXISTS rac_whatsapp_messages_status_check;

ALTER TABLE RAC_whatsapp_messages
    ADD CONSTRAINT rac_whatsapp_messages_status_check
    CHECK (status IN ('received', 'sent', 'failed'));

ALTER TABLE RAC_whatsapp_conversations
    DROP CONSTRAINT IF EXISTS rac_whatsapp_conversations_last_message_status_check;

ALTER TABLE RAC_whatsapp_conversations
    ADD CONSTRAINT rac_whatsapp_conversations_last_message_status_check
    CHECK (last_message_status IN ('received', 'sent', 'failed'));