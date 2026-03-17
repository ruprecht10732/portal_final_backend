-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_user_imap_outbound_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES RAC_user_imap_accounts(id) ON DELETE CASCADE,
    to_addresses TEXT[] NOT NULL DEFAULT '{}',
    cc_addresses TEXT[] NOT NULL DEFAULT '{}',
    from_name TEXT NULL,
    from_address TEXT NOT NULL,
    subject TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NULL,
    sent_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('pending', 'sent', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_user_imap_outbound_messages_account_created_at
    ON RAC_user_imap_outbound_messages(account_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_user_imap_outbound_messages;