package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultListPage        = 1
	defaultListPageSize    = 25
	errIMAPAccountNotFound = "imap account not found"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type Account struct {
	ID                    uuid.UUID
	UserID                uuid.UUID
	EmailAddress          string
	IMAPHost              string
	IMAPPort              int
	IMAPUsername          string
	IMAPPasswordEncrypted string
	SMTPHost              *string
	SMTPPort              *int
	SMTPUsername          *string
	SMTPPasswordEncrypted *string
	SMTPFromEmail         *string
	SMTPFromName          *string
	FolderName            string
	Enabled               bool
	LastSyncAt            *time.Time
	LastError             *string
	LastErrorAt           *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateAccountInput struct {
	UserID                uuid.UUID
	EmailAddress          string
	IMAPHost              string
	IMAPPort              int
	IMAPUsername          string
	IMAPPasswordEncrypted string
	SMTPHost              *string
	SMTPPort              *int
	SMTPUsername          *string
	SMTPPasswordEncrypted *string
	SMTPFromEmail         *string
	SMTPFromName          *string
	FolderName            string
	Enabled               bool
}

type UpdateAccountInput struct {
	EmailAddress          *string
	IMAPHost              *string
	IMAPPort              *int
	IMAPUsername          *string
	IMAPPasswordEncrypted *string
	SMTPHost              *string
	SMTPPort              *int
	SMTPUsername          *string
	SMTPPasswordEncrypted *string
	SMTPFromEmail         *string
	SMTPFromName          *string
	FolderName            *string
	Enabled               *bool
}

type Message struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	FolderName     string
	UID            int64
	MessageID      *string
	FromName       *string
	FromAddress    *string
	Subject        string
	SentAt         *time.Time
	ReceivedAt     *time.Time
	Snippet        *string
	SizeBytes      int64
	Seen           bool
	Flagged        bool
	Answered       bool
	Deleted        bool
	HasAttachments bool
	SyncedAt       time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type UpsertMessageInput struct {
	AccountID      uuid.UUID
	FolderName     string
	UID            int64
	MessageID      *string
	FromName       *string
	FromAddress    *string
	Subject        string
	SentAt         *time.Time
	ReceivedAt     *time.Time
	Snippet        *string
	SizeBytes      int64
	Seen           bool
	Flagged        bool
	Answered       bool
	Deleted        bool
	HasAttachments bool
	SyncedAt       time.Time
}

type ListMessagesParams struct {
	UserID    uuid.UUID
	AccountID uuid.UUID
	Page      int
	PageSize  int
}

type ListMessagesResult struct {
	Items      []Message
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

func (r *Repository) CreateAccount(ctx context.Context, input CreateAccountInput) (Account, error) {
	var account Account
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_user_imap_accounts (
			user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING
			id, user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled, last_sync_at, last_error, last_error_at, created_at, updated_at
	`, input.UserID, input.EmailAddress, input.IMAPHost, input.IMAPPort, input.IMAPUsername, input.IMAPPasswordEncrypted,
		input.SMTPHost, input.SMTPPort, input.SMTPUsername, input.SMTPPasswordEncrypted, input.SMTPFromEmail, input.SMTPFromName,
		input.FolderName, input.Enabled).Scan(
		&account.ID,
		&account.UserID,
		&account.EmailAddress,
		&account.IMAPHost,
		&account.IMAPPort,
		&account.IMAPUsername,
		&account.IMAPPasswordEncrypted,
		&account.SMTPHost,
		&account.SMTPPort,
		&account.SMTPUsername,
		&account.SMTPPasswordEncrypted,
		&account.SMTPFromEmail,
		&account.SMTPFromName,
		&account.FolderName,
		&account.Enabled,
		&account.LastSyncAt,
		&account.LastError,
		&account.LastErrorAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return Account{}, fmt.Errorf("create imap account: %w", err)
	}
	return account, nil
}

func (r *Repository) ListAccountsByUser(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled, last_sync_at, last_error, last_error_at, created_at, updated_at
		FROM RAC_user_imap_accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list imap accounts: %w", err)
	}
	defer rows.Close()

	items := make([]Account, 0)
	for rows.Next() {
		var account Account
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.EmailAddress,
			&account.IMAPHost,
			&account.IMAPPort,
			&account.IMAPUsername,
			&account.IMAPPasswordEncrypted,
			&account.SMTPHost,
			&account.SMTPPort,
			&account.SMTPUsername,
			&account.SMTPPasswordEncrypted,
			&account.SMTPFromEmail,
			&account.SMTPFromName,
			&account.FolderName,
			&account.Enabled,
			&account.LastSyncAt,
			&account.LastError,
			&account.LastErrorAt,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan imap account: %w", err)
		}
		items = append(items, account)
	}
	return items, rows.Err()
}

func (r *Repository) GetAccountByUser(ctx context.Context, accountID, userID uuid.UUID) (Account, error) {
	var account Account
	err := r.pool.QueryRow(ctx, `
		SELECT
			id, user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled, last_sync_at, last_error, last_error_at, created_at, updated_at
		FROM RAC_user_imap_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID).Scan(
		&account.ID,
		&account.UserID,
		&account.EmailAddress,
		&account.IMAPHost,
		&account.IMAPPort,
		&account.IMAPUsername,
		&account.IMAPPasswordEncrypted,
		&account.SMTPHost,
		&account.SMTPPort,
		&account.SMTPUsername,
		&account.SMTPPasswordEncrypted,
		&account.SMTPFromEmail,
		&account.SMTPFromName,
		&account.FolderName,
		&account.Enabled,
		&account.LastSyncAt,
		&account.LastError,
		&account.LastErrorAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, apperr.NotFound(errIMAPAccountNotFound)
	}
	if err != nil {
		return Account{}, fmt.Errorf("get imap account: %w", err)
	}
	return account, nil
}

func (r *Repository) UpdateAccountByUser(ctx context.Context, accountID, userID uuid.UUID, input UpdateAccountInput) (Account, error) {
	var account Account
	err := r.pool.QueryRow(ctx, `
		UPDATE RAC_user_imap_accounts
		SET
			email_address = COALESCE($3, email_address),
			imap_host = COALESCE($4, imap_host),
			imap_port = COALESCE($5, imap_port),
			imap_username = COALESCE($6, imap_username),
			imap_password_encrypted = COALESCE($7, imap_password_encrypted),
			smtp_host = COALESCE($8, smtp_host),
			smtp_port = COALESCE($9, smtp_port),
			smtp_username = COALESCE($10, smtp_username),
			smtp_password_encrypted = COALESCE($11, smtp_password_encrypted),
			smtp_from_email = COALESCE($12, smtp_from_email),
			smtp_from_name = COALESCE($13, smtp_from_name),
			folder_name = COALESCE($14, folder_name),
			enabled = COALESCE($15, enabled),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING
			id, user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled, last_sync_at, last_error, last_error_at, created_at, updated_at
	`, accountID, userID, input.EmailAddress, input.IMAPHost, input.IMAPPort, input.IMAPUsername, input.IMAPPasswordEncrypted,
		input.SMTPHost, input.SMTPPort, input.SMTPUsername, input.SMTPPasswordEncrypted, input.SMTPFromEmail, input.SMTPFromName,
		input.FolderName, input.Enabled).Scan(
		&account.ID,
		&account.UserID,
		&account.EmailAddress,
		&account.IMAPHost,
		&account.IMAPPort,
		&account.IMAPUsername,
		&account.IMAPPasswordEncrypted,
		&account.SMTPHost,
		&account.SMTPPort,
		&account.SMTPUsername,
		&account.SMTPPasswordEncrypted,
		&account.SMTPFromEmail,
		&account.SMTPFromName,
		&account.FolderName,
		&account.Enabled,
		&account.LastSyncAt,
		&account.LastError,
		&account.LastErrorAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, apperr.NotFound(errIMAPAccountNotFound)
	}
	if err != nil {
		return Account{}, fmt.Errorf("update imap account: %w", err)
	}
	return account, nil
}

func (r *Repository) DeleteAccountByUser(ctx context.Context, accountID, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM RAC_user_imap_accounts WHERE id = $1 AND user_id = $2`, accountID, userID)
	if err != nil {
		return fmt.Errorf("delete imap account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound(errIMAPAccountNotFound)
	}
	return nil
}

func (r *Repository) UpsertMessages(ctx context.Context, inputs []UpsertMessageInput) error {
	if len(inputs) == 0 {
		return nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin message upsert tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	for _, input := range inputs {
		_, execErr := tx.Exec(ctx, `
			INSERT INTO RAC_user_imap_messages (
				account_id, folder_name, uid, message_id, from_name, from_address, subject, sent_at, received_at,
				snippet, size_bytes, seen, flagged, answered, deleted, has_attachments, synced_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9,
				$10, $11, $12, $13, $14, $15, $16, $17
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
				updated_at = now()
		`, input.AccountID, input.FolderName, input.UID, input.MessageID, input.FromName, input.FromAddress, input.Subject, input.SentAt, input.ReceivedAt, input.Snippet, input.SizeBytes, input.Seen, input.Flagged, input.Answered, input.Deleted, input.HasAttachments, input.SyncedAt)
		if execErr != nil {
			return fmt.Errorf("upsert message: %w", execErr)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE RAC_user_imap_accounts
		SET last_sync_at = now(), last_error = NULL, last_error_at = NULL, updated_at = now()
		WHERE id = $1
	`, inputs[0].AccountID); err != nil {
		return fmt.Errorf("update account sync state: %w", err)
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("commit message upsert tx: %w", commitErr)
	}
	tx = nil
	return nil
}

func (r *Repository) SetAccountSyncError(ctx context.Context, accountID uuid.UUID, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_user_imap_accounts
		SET last_error = $2, last_error_at = now(), updated_at = now()
		WHERE id = $1
	`, accountID, errMsg)
	if err != nil {
		return fmt.Errorf("set account sync error: %w", err)
	}
	return nil
}

func (r *Repository) MarkAccountSynced(ctx context.Context, accountID uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_user_imap_accounts
		SET last_sync_at = $2, last_error = NULL, last_error_at = NULL, updated_at = now()
		WHERE id = $1
	`, accountID, at)
	if err != nil {
		return fmt.Errorf("mark account synced: %w", err)
	}
	return nil
}

func (r *Repository) ClearAccountSyncError(ctx context.Context, accountID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_user_imap_accounts
		SET last_error = NULL, last_error_at = NULL, updated_at = now()
		WHERE id = $1
	`, accountID)
	if err != nil {
		return fmt.Errorf("clear account sync error: %w", err)
	}
	return nil
}

func (r *Repository) ListMessagesByUser(ctx context.Context, params ListMessagesParams) (ListMessagesResult, error) {
	page := params.Page
	if page < 1 {
		page = defaultListPage
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = defaultListPageSize
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(m.id)
		FROM RAC_user_imap_messages m
		JOIN RAC_user_imap_accounts a ON a.id = m.account_id
		WHERE m.account_id = $1 AND a.user_id = $2
	`, params.AccountID, params.UserID).Scan(&total); err != nil {
		return ListMessagesResult{}, fmt.Errorf("count imap messages: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, `
		SELECT
			m.id, m.account_id, m.folder_name, m.uid, m.message_id, m.from_name, m.from_address, m.subject,
			m.sent_at, m.received_at, m.snippet, m.size_bytes, m.seen, m.flagged, m.answered, m.deleted,
			m.has_attachments, m.synced_at, m.created_at, m.updated_at
		FROM RAC_user_imap_messages m
		JOIN RAC_user_imap_accounts a ON a.id = m.account_id
		WHERE m.account_id = $1 AND a.user_id = $2
		ORDER BY COALESCE(m.sent_at, m.received_at, m.created_at) DESC
		LIMIT $3 OFFSET $4
	`, params.AccountID, params.UserID, pageSize, offset)
	if err != nil {
		return ListMessagesResult{}, fmt.Errorf("list imap messages: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0)
	for rows.Next() {
		var item Message
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.FolderName, &item.UID, &item.MessageID, &item.FromName, &item.FromAddress, &item.Subject,
			&item.SentAt, &item.ReceivedAt, &item.Snippet, &item.SizeBytes, &item.Seen, &item.Flagged, &item.Answered, &item.Deleted,
			&item.HasAttachments, &item.SyncedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return ListMessagesResult{}, fmt.Errorf("scan imap message: %w", err)
		}
		items = append(items, item)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	return ListMessagesResult{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, rows.Err()
}

// CountUnreadMessagesByUser returns the exact unread IMAP message count across all accounts for a user.
func (r *Repository) CountUnreadMessagesByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(m.id)
		FROM RAC_user_imap_messages m
		JOIN RAC_user_imap_accounts a ON a.id = m.account_id
		WHERE a.user_id = $1
		  AND m.seen = false
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread imap messages: %w", err)
	}
	return count, nil
}

func (r *Repository) DeleteMessageMetadataByUID(ctx context.Context, accountID uuid.UUID, uid int64) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM RAC_user_imap_messages
		WHERE account_id = $1 AND uid = $2
	`, accountID, uid)
	if err != nil {
		return fmt.Errorf("delete message metadata: %w", err)
	}
	return nil
}

func (r *Repository) UpdateMessageSeenByUID(ctx context.Context, accountID uuid.UUID, uid int64, seen bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_user_imap_messages
		SET seen = $3, updated_at = now()
		WHERE account_id = $1 AND uid = $2
	`, accountID, uid, seen)
	if err != nil {
		return fmt.Errorf("update message seen metadata: %w", err)
	}
	return nil
}

func (r *Repository) UpdateMessageAnsweredByUID(ctx context.Context, accountID uuid.UUID, uid int64, answered bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE RAC_user_imap_messages
		SET answered = $3, updated_at = now()
		WHERE account_id = $1 AND uid = $2
	`, accountID, uid, answered)
	if err != nil {
		return fmt.Errorf("update message answered metadata: %w", err)
	}
	return nil
}

func (r *Repository) GetMessageSizeByUID(ctx context.Context, accountID uuid.UUID, uid int64) (int64, error) {
	var sizeBytes int64
	err := r.pool.QueryRow(ctx, `
		SELECT size_bytes
		FROM RAC_user_imap_messages
		WHERE account_id = $1 AND uid = $2
		LIMIT 1
	`, accountID, uid).Scan(&sizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get imap message size by uid: %w", err)
	}
	return sizeBytes, nil
}

// GetMaxUID returns the highest UID currently synced for a given account and folder.
func (r *Repository) GetMaxUID(ctx context.Context, accountID uuid.UUID, folderName string) (int64, error) {
	var maxUID *int64
	err := r.pool.QueryRow(ctx, `
		SELECT MAX(uid)
		FROM RAC_user_imap_messages
		WHERE account_id = $1 AND folder_name = $2
	`, accountID, folderName).Scan(&maxUID)
	if err != nil {
		return 0, fmt.Errorf("get max imap uid: %w", err)
	}
	if maxUID == nil {
		return 0, nil
	}
	return *maxUID, nil
}

func (r *Repository) ListAccountsNeedingSync(ctx context.Context, maxAge time.Duration, limit int) ([]Account, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, user_id, email_address, imap_host, imap_port, imap_username, imap_password_encrypted,
			smtp_host, smtp_port, smtp_username, smtp_password_encrypted, smtp_from_email, smtp_from_name,
			folder_name, enabled, last_sync_at, last_error, last_error_at, created_at, updated_at
		FROM RAC_user_imap_accounts
		WHERE enabled = true
		  AND (last_sync_at IS NULL OR last_sync_at <= now() - $1::interval)
		ORDER BY COALESCE(last_sync_at, created_at) ASC
		LIMIT $2
	`, fmt.Sprintf("%f seconds", maxAge.Seconds()), limit)
	if err != nil {
		return nil, fmt.Errorf("list accounts needing sync: %w", err)
	}
	defer rows.Close()

	items := make([]Account, 0)
	for rows.Next() {
		var account Account
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.EmailAddress,
			&account.IMAPHost,
			&account.IMAPPort,
			&account.IMAPUsername,
			&account.IMAPPasswordEncrypted,
			&account.SMTPHost,
			&account.SMTPPort,
			&account.SMTPUsername,
			&account.SMTPPasswordEncrypted,
			&account.SMTPFromEmail,
			&account.SMTPFromName,
			&account.FolderName,
			&account.Enabled,
			&account.LastSyncAt,
			&account.LastError,
			&account.LastErrorAt,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account needing sync: %w", err)
		}
		items = append(items, account)
	}
	return items, rows.Err()
}
