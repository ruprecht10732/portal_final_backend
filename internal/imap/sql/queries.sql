-- name: CreateImapAccount :one
INSERT INTO RAC_user_imap_accounts (
	user_id,
	email_address,
	imap_host,
	imap_port,
	imap_username,
	imap_password_encrypted,
	smtp_host,
	smtp_port,
	smtp_username,
	smtp_password_encrypted,
	smtp_from_email,
	smtp_from_name,
	folder_name,
	enabled
) VALUES (
	sqlc.arg(user_id)::uuid,
	sqlc.arg(email_address)::text,
	sqlc.arg(imap_host)::text,
	sqlc.arg(imap_port)::int,
	sqlc.arg(imap_username)::text,
	sqlc.arg(imap_password_encrypted)::text,
	sqlc.narg(smtp_host)::text,
	sqlc.narg(smtp_port)::int,
	sqlc.narg(smtp_username)::text,
	sqlc.narg(smtp_password_encrypted)::text,
	sqlc.narg(smtp_from_email)::text,
	sqlc.narg(smtp_from_name)::text,
	sqlc.arg(folder_name)::text,
	sqlc.arg(enabled)::bool
)
RETURNING *;

-- name: ListImapAccountsByUser :many
SELECT *
FROM RAC_user_imap_accounts
WHERE user_id = sqlc.arg(user_id)::uuid
ORDER BY created_at DESC;

-- name: GetImapAccountByUser :one
SELECT *
FROM RAC_user_imap_accounts
WHERE id = sqlc.arg(account_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: UpdateImapAccountByUser :one
UPDATE RAC_user_imap_accounts
SET email_address = COALESCE(sqlc.narg(email_address)::text, email_address),
	imap_host = COALESCE(sqlc.narg(imap_host)::text, imap_host),
	imap_port = COALESCE(sqlc.narg(imap_port)::int, imap_port),
	imap_username = COALESCE(sqlc.narg(imap_username)::text, imap_username),
	imap_password_encrypted = COALESCE(sqlc.narg(imap_password_encrypted)::text, imap_password_encrypted),
	smtp_host = COALESCE(sqlc.narg(smtp_host)::text, smtp_host),
	smtp_port = COALESCE(sqlc.narg(smtp_port)::int, smtp_port),
	smtp_username = COALESCE(sqlc.narg(smtp_username)::text, smtp_username),
	smtp_password_encrypted = COALESCE(sqlc.narg(smtp_password_encrypted)::text, smtp_password_encrypted),
	smtp_from_email = COALESCE(sqlc.narg(smtp_from_email)::text, smtp_from_email),
	smtp_from_name = COALESCE(sqlc.narg(smtp_from_name)::text, smtp_from_name),
	folder_name = COALESCE(sqlc.narg(folder_name)::text, folder_name),
	enabled = COALESCE(sqlc.narg(enabled)::bool, enabled),
	updated_at = now()
WHERE id = sqlc.arg(account_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid
RETURNING *;

-- name: DeleteImapAccountByUser :execrows
DELETE FROM RAC_user_imap_accounts
WHERE id = sqlc.arg(account_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: UpsertImapMessage :exec
INSERT INTO RAC_user_imap_messages (
	account_id,
	folder_name,
	uid,
	message_id,
	from_name,
	from_address,
	subject,
	sent_at,
	received_at,
	snippet,
	size_bytes,
	seen,
	flagged,
	answered,
	deleted,
	has_attachments,
	synced_at
) VALUES (
	sqlc.arg(account_id)::uuid,
	sqlc.arg(folder_name)::text,
	sqlc.arg(uid)::bigint,
	sqlc.narg(message_id)::text,
	sqlc.narg(from_name)::text,
	sqlc.narg(from_address)::text,
	sqlc.arg(subject)::text,
	sqlc.narg(sent_at)::timestamptz,
	sqlc.narg(received_at)::timestamptz,
	sqlc.narg(snippet)::text,
	sqlc.arg(size_bytes)::bigint,
	sqlc.arg(seen)::bool,
	sqlc.arg(flagged)::bool,
	sqlc.arg(answered)::bool,
	sqlc.arg(deleted)::bool,
	sqlc.arg(has_attachments)::bool,
	sqlc.arg(synced_at)::timestamptz
)
ON CONFLICT (account_id, folder_name, uid)
DO UPDATE SET
	message_id = EXCLUDED.message_id,
	from_name = EXCLUDED.from_name,
	from_address = EXCLUDED.from_address,
	subject = EXCLUDED.subject,
	sent_at = EXCLUDED.sent_at,
	received_at = EXCLUDED.received_at,
	snippet = EXCLUDED.snippet,
	size_bytes = EXCLUDED.size_bytes,
	seen = EXCLUDED.seen,
	flagged = EXCLUDED.flagged,
	answered = EXCLUDED.answered,
	deleted = EXCLUDED.deleted,
	has_attachments = EXCLUDED.has_attachments,
	synced_at = EXCLUDED.synced_at,
	updated_at = now();

-- name: MarkImapAccountMessagesSynced :exec
UPDATE RAC_user_imap_accounts
SET last_sync_at = now(),
	last_error = NULL,
	last_error_at = NULL,
	updated_at = now()
WHERE id = sqlc.arg(account_id)::uuid;

-- name: SetImapAccountSyncError :exec
UPDATE RAC_user_imap_accounts
SET last_error = sqlc.arg(error_message)::text,
	last_error_at = now(),
	updated_at = now()
WHERE id = sqlc.arg(account_id)::uuid;

-- name: MarkImapAccountSynced :exec
UPDATE RAC_user_imap_accounts
SET last_sync_at = sqlc.arg(sync_at)::timestamptz,
	last_error = NULL,
	last_error_at = NULL,
	updated_at = now()
WHERE id = sqlc.arg(account_id)::uuid;

-- name: ClearImapAccountSyncError :exec
UPDATE RAC_user_imap_accounts
SET last_error = NULL,
	last_error_at = NULL,
	updated_at = now()
WHERE id = sqlc.arg(account_id)::uuid;

-- name: CountImapMessagesByUserAndAccount :one
SELECT COUNT(m.id)::bigint
FROM RAC_user_imap_messages m
JOIN RAC_user_imap_accounts a ON a.id = m.account_id
WHERE m.account_id = sqlc.arg(account_id)::uuid
  AND a.user_id = sqlc.arg(user_id)::uuid;

-- name: ListImapMessagesByUser :many
SELECT m.*
FROM RAC_user_imap_messages m
JOIN RAC_user_imap_accounts a ON a.id = m.account_id
WHERE m.account_id = sqlc.arg(account_id)::uuid
  AND a.user_id = sqlc.arg(user_id)::uuid
ORDER BY COALESCE(m.sent_at, m.received_at, m.created_at) DESC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: CountUnreadImapMessagesByUser :one
SELECT COUNT(m.id)::bigint
FROM RAC_user_imap_messages m
JOIN RAC_user_imap_accounts a ON a.id = m.account_id
WHERE a.user_id = sqlc.arg(user_id)::uuid
  AND m.seen = FALSE;

-- name: DeleteImapMessageMetadataByUID :exec
DELETE FROM RAC_user_imap_messages
WHERE account_id = sqlc.arg(account_id)::uuid
  AND uid = sqlc.arg(uid)::bigint;

-- name: UpdateImapMessageSeenByUID :exec
UPDATE RAC_user_imap_messages
SET seen = sqlc.arg(seen)::bool,
	updated_at = now()
WHERE account_id = sqlc.arg(account_id)::uuid
  AND uid = sqlc.arg(uid)::bigint;

-- name: UpdateImapMessageAnsweredByUID :exec
UPDATE RAC_user_imap_messages
SET answered = sqlc.arg(answered)::bool,
	updated_at = now()
WHERE account_id = sqlc.arg(account_id)::uuid
  AND uid = sqlc.arg(uid)::bigint;

-- name: GetImapMessageSizeByUID :one
SELECT size_bytes
FROM RAC_user_imap_messages
WHERE account_id = sqlc.arg(account_id)::uuid
  AND uid = sqlc.arg(uid)::bigint
LIMIT 1;

-- name: GetImapMaxUID :one
SELECT COALESCE(MAX(uid), 0)::bigint
FROM RAC_user_imap_messages
WHERE account_id = sqlc.arg(account_id)::uuid
  AND folder_name = sqlc.arg(folder_name)::text;

-- name: ListImapAccountsNeedingSync :many
SELECT *
FROM RAC_user_imap_accounts
WHERE enabled = TRUE
  AND (last_sync_at IS NULL OR last_sync_at <= now() - sqlc.arg(sync_age)::interval)
ORDER BY COALESCE(last_sync_at, created_at) ASC
LIMIT sqlc.arg(limit_count)::int;