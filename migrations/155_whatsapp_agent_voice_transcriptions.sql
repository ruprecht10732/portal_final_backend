-- +goose Up

CREATE TABLE RAC_whatsapp_agent_voice_transcriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id),
    external_message_id TEXT NOT NULL,
    phone_number TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    storage_bucket TEXT,
    storage_key TEXT,
    content_type TEXT,
    provider TEXT,
    language TEXT,
    confidence_score DOUBLE PRECISION,
    transcript_text TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_wa_agent_voice_transcriptions_org_external
    ON RAC_whatsapp_agent_voice_transcriptions (organization_id, external_message_id);

CREATE INDEX idx_wa_agent_voice_transcriptions_status
    ON RAC_whatsapp_agent_voice_transcriptions (status, updated_at DESC);

-- +goose Down

DROP TABLE IF EXISTS RAC_whatsapp_agent_voice_transcriptions;