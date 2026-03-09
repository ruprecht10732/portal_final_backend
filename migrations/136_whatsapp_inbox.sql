-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_whatsapp_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_id UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,
    phone_number TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    last_message_preview TEXT NOT NULL DEFAULT '',
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_direction TEXT NOT NULL DEFAULT 'outbound',
    last_message_status TEXT NOT NULL DEFAULT 'sent',
    unread_count INTEGER NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, phone_number),
    CHECK (last_message_direction IN ('inbound', 'outbound')),
    CHECK (last_message_status IN ('received', 'sent', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_conversations_org_last_message_at
    ON RAC_whatsapp_conversations (organization_id, last_message_at DESC, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_whatsapp_conversations_org_lead
    ON RAC_whatsapp_conversations (organization_id, lead_id)
    WHERE lead_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS RAC_whatsapp_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES RAC_whatsapp_conversations(id) ON DELETE CASCADE,
    lead_id UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,
    external_message_id TEXT,
    direction TEXT NOT NULL,
    status TEXT NOT NULL,
    phone_number TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    sent_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (direction IN ('inbound', 'outbound')),
    CHECK (status IN ('received', 'sent', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_messages_conversation_created_at
    ON RAC_whatsapp_messages (conversation_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_whatsapp_messages_org_created_at
    ON RAC_whatsapp_messages (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_whatsapp_messages_unread_inbound
    ON RAC_whatsapp_messages (conversation_id)
    WHERE direction = 'inbound' AND read_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_whatsapp_messages_unread_inbound;
DROP INDEX IF EXISTS idx_whatsapp_messages_org_created_at;
DROP INDEX IF EXISTS idx_whatsapp_messages_conversation_created_at;
DROP TABLE IF EXISTS RAC_whatsapp_messages;
DROP INDEX IF EXISTS idx_whatsapp_conversations_org_lead;
DROP INDEX IF EXISTS idx_whatsapp_conversations_org_last_message_at;
DROP TABLE IF EXISTS RAC_whatsapp_conversations;