-- +goose Up
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

-- +goose Down
DROP INDEX IF EXISTS idx_user_imap_message_leads_account_uid;
DROP INDEX IF EXISTS idx_user_imap_message_leads_org_lead;
DROP TABLE IF EXISTS RAC_user_imap_message_leads;