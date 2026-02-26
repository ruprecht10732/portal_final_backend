-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_user_imap_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    email_address TEXT NOT NULL,
    imap_host TEXT NOT NULL,
    imap_port INTEGER NOT NULL CHECK (imap_port > 0),
    imap_username TEXT NOT NULL,
    imap_password_encrypted TEXT NOT NULL,
    folder_name TEXT NOT NULL DEFAULT 'INBOX',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,
    last_error_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, email_address)
);

CREATE INDEX IF NOT EXISTS idx_user_imap_accounts_user_id ON RAC_user_imap_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_user_imap_accounts_enabled_last_sync ON RAC_user_imap_accounts(enabled, last_sync_at);

CREATE TABLE IF NOT EXISTS RAC_user_imap_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES RAC_user_imap_accounts(id) ON DELETE CASCADE,
    folder_name TEXT NOT NULL,
    uid BIGINT NOT NULL,
    message_id TEXT NULL,
    from_name TEXT NULL,
    from_address TEXT NULL,
    subject TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ NULL,
    received_at TIMESTAMPTZ NULL,
    snippet TEXT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    seen BOOLEAN NOT NULL DEFAULT FALSE,
    flagged BOOLEAN NOT NULL DEFAULT FALSE,
    answered BOOLEAN NOT NULL DEFAULT FALSE,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    has_attachments BOOLEAN NOT NULL DEFAULT FALSE,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, folder_name, uid)
);

CREATE INDEX IF NOT EXISTS idx_user_imap_messages_account_sent_at ON RAC_user_imap_messages(account_id, sent_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_user_imap_messages;
DROP TABLE IF EXISTS RAC_user_imap_accounts;
