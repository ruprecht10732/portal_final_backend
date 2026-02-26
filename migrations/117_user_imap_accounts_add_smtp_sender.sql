-- +goose Up
ALTER TABLE RAC_user_imap_accounts
    ADD COLUMN IF NOT EXISTS smtp_host TEXT NULL,
    ADD COLUMN IF NOT EXISTS smtp_port INTEGER NULL CHECK (smtp_port > 0),
    ADD COLUMN IF NOT EXISTS smtp_username TEXT NULL,
    ADD COLUMN IF NOT EXISTS smtp_password_encrypted TEXT NULL,
    ADD COLUMN IF NOT EXISTS smtp_from_email TEXT NULL,
    ADD COLUMN IF NOT EXISTS smtp_from_name TEXT NULL;

UPDATE RAC_user_imap_accounts
SET
    smtp_host = COALESCE(smtp_host, imap_host),
    smtp_port = COALESCE(smtp_port, 587),
    smtp_username = COALESCE(smtp_username, imap_username),
    smtp_password_encrypted = COALESCE(smtp_password_encrypted, imap_password_encrypted),
    smtp_from_email = COALESCE(smtp_from_email, email_address)
WHERE smtp_host IS NULL
   OR smtp_port IS NULL
   OR smtp_username IS NULL
   OR smtp_password_encrypted IS NULL
   OR smtp_from_email IS NULL;

-- +goose Down
ALTER TABLE RAC_user_imap_accounts
    DROP COLUMN IF EXISTS smtp_from_name,
    DROP COLUMN IF EXISTS smtp_from_email,
    DROP COLUMN IF EXISTS smtp_password_encrypted,
    DROP COLUMN IF EXISTS smtp_username,
    DROP COLUMN IF EXISTS smtp_port,
    DROP COLUMN IF EXISTS smtp_host;
