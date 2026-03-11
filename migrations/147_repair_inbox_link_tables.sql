-- +goose Up
-- Repair migration for environments where older databases were bootstrapped
-- with goose history but the inbox link tables were not actually created.

CREATE TABLE IF NOT EXISTS RAC_user_imap_message_leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES RAC_user_imap_accounts(id) ON DELETE CASCADE,
    message_uid BIGINT NOT NULL,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, message_uid)
);

CREATE INDEX IF NOT EXISTS idx_user_imap_message_leads_org_lead
    ON RAC_user_imap_message_leads (organization_id, lead_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_imap_message_leads_account_uid
    ON RAC_user_imap_message_leads (account_id, message_uid);

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
SELECT 1;