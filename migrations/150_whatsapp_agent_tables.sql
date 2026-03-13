-- +goose Up

CREATE TABLE RAC_whatsapp_agent_users (
    phone_number TEXT PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id),
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE RAC_organization_invite_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id),
    code TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_invite_code UNIQUE (code)
);
CREATE INDEX idx_invite_codes_code ON RAC_organization_invite_codes (code) WHERE is_active = true;

CREATE TABLE RAC_whatsapp_agent_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    phone_number TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wa_agent_messages_phone_time ON RAC_whatsapp_agent_messages (phone_number, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_whatsapp_agent_messages;
DROP TABLE IF EXISTS RAC_organization_invite_codes;
DROP TABLE IF EXISTS RAC_whatsapp_agent_users;
