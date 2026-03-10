-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_whatsapp_reply_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES RAC_whatsapp_conversations(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    ai_reply TEXT NOT NULL,
    human_reply TEXT NOT NULL,
    applied_to_memory BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rac_whatsapp_reply_feedback_org_service_created
    ON RAC_whatsapp_reply_feedback (organization_id, lead_service_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_rac_whatsapp_reply_feedback_org_conversation_created
    ON RAC_whatsapp_reply_feedback (organization_id, conversation_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_rac_whatsapp_reply_feedback_org_conversation_created;
DROP INDEX IF EXISTS idx_rac_whatsapp_reply_feedback_org_service_created;
DROP TABLE IF EXISTS RAC_whatsapp_reply_feedback;