-- +goose Up

-- Add superadmin role for global agent device management
INSERT INTO RAC_roles (name) VALUES ('superadmin') ON CONFLICT (name) DO NOTHING;

-- Global WhatsApp agent device config (singleton row expected)
CREATE TABLE RAC_whatsapp_agent_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id TEXT NOT NULL UNIQUE,
    account_jid TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DELETE FROM RAC_roles WHERE name = 'superadmin';
DROP TABLE IF EXISTS RAC_whatsapp_agent_config;
