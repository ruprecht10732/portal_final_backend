-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_email_reply_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES RAC_user_imap_accounts(id) ON DELETE CASCADE,
    source_message_uid BIGINT NOT NULL,
    customer_email TEXT NOT NULL,
    customer_name TEXT NULL,
    subject TEXT NULL,
    customer_message TEXT NOT NULL,
    ai_reply TEXT NULL,
    human_reply TEXT NOT NULL,
    reply_all BOOLEAN NOT NULL DEFAULT FALSE,
    lead_id UUID NULL REFERENCES RAC_leads(id) ON DELETE SET NULL,
    lead_service_id UUID NULL REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    applied_to_memory BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rac_email_reply_feedback_org_customer_created
    ON RAC_email_reply_feedback (organization_id, customer_email, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_rac_email_reply_feedback_org_service_created
    ON RAC_email_reply_feedback (organization_id, lead_service_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_rac_email_reply_feedback_org_service_created;
DROP INDEX IF EXISTS idx_rac_email_reply_feedback_org_customer_created;
DROP TABLE IF EXISTS RAC_email_reply_feedback;