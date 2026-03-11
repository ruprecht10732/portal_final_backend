package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	imapdb "portal_final_backend/internal/imap/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultListPage        = 1
	defaultListPageSize    = 25
	errIMAPAccountNotFound = "imap account not found"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *imapdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	if pool == nil {
		return &Repository{}
	}
	return &Repository{pool: pool, queries: imapdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgInt4Ptr(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

func toPgBoolPtr(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return toPgTimestamp(*value)
}

func toPgInterval(value time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: value.Microseconds(), Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalInt(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int32)
	return &n
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func accountFromModel(model imapdb.RacUserImapAccount) Account {
	return Account{
		ID:                    uuid.UUID(model.ID.Bytes),
		UserID:                uuid.UUID(model.UserID.Bytes),
		EmailAddress:          model.EmailAddress,
		IMAPHost:              model.ImapHost,
		IMAPPort:              int(model.ImapPort),
		IMAPUsername:          model.ImapUsername,
		IMAPPasswordEncrypted: model.ImapPasswordEncrypted,
		SMTPHost:              optionalString(model.SmtpHost),
		SMTPPort:              optionalInt(model.SmtpPort),
		SMTPUsername:          optionalString(model.SmtpUsername),
		SMTPPasswordEncrypted: optionalString(model.SmtpPasswordEncrypted),
		SMTPFromEmail:         optionalString(model.SmtpFromEmail),
		SMTPFromName:          optionalString(model.SmtpFromName),
		FolderName:            model.FolderName,
		Enabled:               model.Enabled,
		LastSyncAt:            optionalTime(model.LastSyncAt),
		LastError:             optionalString(model.LastError),
		LastErrorAt:           optionalTime(model.LastErrorAt),
		CreatedAt:             model.CreatedAt.Time,
		UpdatedAt:             model.UpdatedAt.Time,
	}
}

func messageFromModel(model imapdb.RacUserImapMessage) Message {
	return Message{
		ID:             uuid.UUID(model.ID.Bytes),
		AccountID:      uuid.UUID(model.AccountID.Bytes),
		FolderName:     model.FolderName,
		UID:            model.Uid,
		MessageID:      optionalString(model.MessageID),
		FromName:       optionalString(model.FromName),
		FromAddress:    optionalString(model.FromAddress),
		Subject:        model.Subject,
		SentAt:         optionalTime(model.SentAt),
		ReceivedAt:     optionalTime(model.ReceivedAt),
		Snippet:        optionalString(model.Snippet),
		SizeBytes:      model.SizeBytes,
		Seen:           model.Seen,
		Flagged:        model.Flagged,
		Answered:       model.Answered,
		Deleted:        model.Deleted,
		HasAttachments: model.HasAttachments,
		SyncedAt:       model.SyncedAt.Time,
		CreatedAt:      model.CreatedAt.Time,
		UpdatedAt:      model.UpdatedAt.Time,
	}
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

type MessageLeadLink struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	MessageUID     int64
	LeadID         uuid.UUID
	CreatedBy      uuid.UUID
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
	model, err := r.queries.CreateImapAccount(ctx, imapdb.CreateImapAccountParams{
		UserID:                toPgUUID(input.UserID),
		EmailAddress:          input.EmailAddress,
		ImapHost:              input.IMAPHost,
		ImapPort:              int32(input.IMAPPort),
		ImapUsername:          input.IMAPUsername,
		ImapPasswordEncrypted: input.IMAPPasswordEncrypted,
		SmtpHost:              toPgText(input.SMTPHost),
		SmtpPort:              toPgInt4Ptr(input.SMTPPort),
		SmtpUsername:          toPgText(input.SMTPUsername),
		SmtpPasswordEncrypted: toPgText(input.SMTPPasswordEncrypted),
		SmtpFromEmail:         toPgText(input.SMTPFromEmail),
		SmtpFromName:          toPgText(input.SMTPFromName),
		FolderName:            input.FolderName,
		Enabled:               input.Enabled,
	})
	if err != nil {
		return Account{}, fmt.Errorf("create imap account: %w", err)
	}
	return accountFromModel(model), nil
}

func (r *Repository) ListAccountsByUser(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	rows, err := r.queries.ListImapAccountsByUser(ctx, toPgUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("list imap accounts: %w", err)
	}

	items := make([]Account, 0)
	for _, row := range rows {
		items = append(items, accountFromModel(row))
	}
	return items, nil
}

func (r *Repository) GetAccountByUser(ctx context.Context, accountID, userID uuid.UUID) (Account, error) {
	model, err := r.queries.GetImapAccountByUser(ctx, imapdb.GetImapAccountByUserParams{
		AccountID: toPgUUID(accountID),
		UserID:    toPgUUID(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, apperr.NotFound(errIMAPAccountNotFound)
	}
	if err != nil {
		return Account{}, fmt.Errorf("get imap account: %w", err)
	}
	return accountFromModel(model), nil
}

func (r *Repository) UpdateAccountByUser(ctx context.Context, accountID, userID uuid.UUID, input UpdateAccountInput) (Account, error) {
	model, err := r.queries.UpdateImapAccountByUser(ctx, imapdb.UpdateImapAccountByUserParams{
		EmailAddress:          toPgText(input.EmailAddress),
		ImapHost:              toPgText(input.IMAPHost),
		ImapPort:              toPgInt4Ptr(input.IMAPPort),
		ImapUsername:          toPgText(input.IMAPUsername),
		ImapPasswordEncrypted: toPgText(input.IMAPPasswordEncrypted),
		SmtpHost:              toPgText(input.SMTPHost),
		SmtpPort:              toPgInt4Ptr(input.SMTPPort),
		SmtpUsername:          toPgText(input.SMTPUsername),
		SmtpPasswordEncrypted: toPgText(input.SMTPPasswordEncrypted),
		SmtpFromEmail:         toPgText(input.SMTPFromEmail),
		SmtpFromName:          toPgText(input.SMTPFromName),
		FolderName:            toPgText(input.FolderName),
		Enabled:               toPgBoolPtr(input.Enabled),
		AccountID:             toPgUUID(accountID),
		UserID:                toPgUUID(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, apperr.NotFound(errIMAPAccountNotFound)
	}
	if err != nil {
		return Account{}, fmt.Errorf("update imap account: %w", err)
	}
	return accountFromModel(model), nil
}

func (r *Repository) DeleteAccountByUser(ctx context.Context, accountID, userID uuid.UUID) error {
	tag, err := r.queries.DeleteImapAccountByUser(ctx, imapdb.DeleteImapAccountByUserParams{
		AccountID: toPgUUID(accountID),
		UserID:    toPgUUID(userID),
	})
	if err != nil {
		return fmt.Errorf("delete imap account: %w", err)
	}
	if tag == 0 {
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

	queries := r.queries.WithTx(tx)
	for _, input := range inputs {
		execErr := queries.UpsertImapMessage(ctx, imapdb.UpsertImapMessageParams{
			AccountID:      toPgUUID(input.AccountID),
			FolderName:     input.FolderName,
			Uid:            input.UID,
			MessageID:      toPgText(input.MessageID),
			FromName:       toPgText(input.FromName),
			FromAddress:    toPgText(input.FromAddress),
			Subject:        input.Subject,
			SentAt:         toPgTimestampPtr(input.SentAt),
			ReceivedAt:     toPgTimestampPtr(input.ReceivedAt),
			Snippet:        toPgText(input.Snippet),
			SizeBytes:      input.SizeBytes,
			Seen:           input.Seen,
			Flagged:        input.Flagged,
			Answered:       input.Answered,
			Deleted:        input.Deleted,
			HasAttachments: input.HasAttachments,
			SyncedAt:       toPgTimestamp(input.SyncedAt),
		})
		if execErr != nil {
			return fmt.Errorf("upsert message: %w", execErr)
		}
	}

	if err := queries.MarkImapAccountMessagesSynced(ctx, toPgUUID(inputs[0].AccountID)); err != nil {
		return fmt.Errorf("update account sync state: %w", err)
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("commit message upsert tx: %w", commitErr)
	}
	tx = nil
	return nil
}

func (r *Repository) SetAccountSyncError(ctx context.Context, accountID uuid.UUID, errMsg string) error {
	err := r.queries.SetImapAccountSyncError(ctx, imapdb.SetImapAccountSyncErrorParams{
		ErrorMessage: errMsg,
		AccountID:    toPgUUID(accountID),
	})
	if err != nil {
		return fmt.Errorf("set account sync error: %w", err)
	}
	return nil
}

func (r *Repository) MarkAccountSynced(ctx context.Context, accountID uuid.UUID, at time.Time) error {
	err := r.queries.MarkImapAccountSynced(ctx, imapdb.MarkImapAccountSyncedParams{
		SyncAt:    toPgTimestamp(at),
		AccountID: toPgUUID(accountID),
	})
	if err != nil {
		return fmt.Errorf("mark account synced: %w", err)
	}
	return nil
}

func (r *Repository) ClearAccountSyncError(ctx context.Context, accountID uuid.UUID) error {
	err := r.queries.ClearImapAccountSyncError(ctx, toPgUUID(accountID))
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

	total, err := r.queries.CountImapMessagesByUserAndAccount(ctx, imapdb.CountImapMessagesByUserAndAccountParams{
		AccountID: toPgUUID(params.AccountID),
		UserID:    toPgUUID(params.UserID),
	})
	if err != nil {
		return ListMessagesResult{}, fmt.Errorf("count imap messages: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.queries.ListImapMessagesByUser(ctx, imapdb.ListImapMessagesByUserParams{
		AccountID:   toPgUUID(params.AccountID),
		UserID:      toPgUUID(params.UserID),
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		return ListMessagesResult{}, fmt.Errorf("list imap messages: %w", err)
	}

	items := make([]Message, 0)
	for _, row := range rows {
		items = append(items, messageFromModel(row))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (int(total) + pageSize - 1) / pageSize
	}

	return ListMessagesResult{
		Items:      items,
		Total:      int(total),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// CountUnreadMessagesByUser returns the exact unread IMAP message count across all accounts for a user.
func (r *Repository) CountUnreadMessagesByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := r.queries.CountUnreadImapMessagesByUser(ctx, toPgUUID(userID))
	if err != nil {
		return 0, fmt.Errorf("count unread imap messages: %w", err)
	}
	return int(count), nil
}

func (r *Repository) DeleteMessageMetadataByUID(ctx context.Context, accountID uuid.UUID, uid int64) error {
	err := r.queries.DeleteImapMessageMetadataByUID(ctx, imapdb.DeleteImapMessageMetadataByUIDParams{
		AccountID: toPgUUID(accountID),
		Uid:       uid,
	})
	if err != nil {
		return fmt.Errorf("delete message metadata: %w", err)
	}
	return nil
}

func (r *Repository) UpdateMessageSeenByUID(ctx context.Context, accountID uuid.UUID, uid int64, seen bool) error {
	err := r.queries.UpdateImapMessageSeenByUID(ctx, imapdb.UpdateImapMessageSeenByUIDParams{
		Seen:      seen,
		AccountID: toPgUUID(accountID),
		Uid:       uid,
	})
	if err != nil {
		return fmt.Errorf("update message seen metadata: %w", err)
	}
	return nil
}

func (r *Repository) UpdateMessageAnsweredByUID(ctx context.Context, accountID uuid.UUID, uid int64, answered bool) error {
	err := r.queries.UpdateImapMessageAnsweredByUID(ctx, imapdb.UpdateImapMessageAnsweredByUIDParams{
		Answered:  answered,
		AccountID: toPgUUID(accountID),
		Uid:       uid,
	})
	if err != nil {
		return fmt.Errorf("update message answered metadata: %w", err)
	}
	return nil
}

func (r *Repository) GetMessageSizeByUID(ctx context.Context, accountID uuid.UUID, uid int64) (int64, error) {
	sizeBytes, err := r.queries.GetImapMessageSizeByUID(ctx, imapdb.GetImapMessageSizeByUIDParams{
		AccountID: toPgUUID(accountID),
		Uid:       uid,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get imap message size by uid: %w", err)
	}
	return sizeBytes, nil
}

func (r *Repository) GetMessageByUIDByUser(ctx context.Context, userID, accountID uuid.UUID, uid int64) (Message, error) {
	const query = `
		SELECT m.id, m.account_id, m.folder_name, m.uid, m.message_id, m.from_name, m.from_address,
		       m.subject, m.sent_at, m.received_at, m.snippet, m.size_bytes, m.seen, m.flagged,
		       m.answered, m.deleted, m.has_attachments, m.synced_at, m.created_at, m.updated_at
		FROM RAC_user_imap_messages m
		JOIN RAC_user_imap_accounts a ON a.id = m.account_id
		WHERE a.user_id = $1 AND m.account_id = $2 AND m.uid = $3
		ORDER BY m.updated_at DESC, m.created_at DESC
		LIMIT 1`

	row := r.pool.QueryRow(ctx, query, userID, accountID, uid)
	var item Message
	var messageID pgtype.Text
	var fromName pgtype.Text
	var fromAddress pgtype.Text
	var sentAt pgtype.Timestamptz
	var receivedAt pgtype.Timestamptz
	var snippet pgtype.Text
	if err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.FolderName,
		&item.UID,
		&messageID,
		&fromName,
		&fromAddress,
		&item.Subject,
		&sentAt,
		&receivedAt,
		&snippet,
		&item.SizeBytes,
		&item.Seen,
		&item.Flagged,
		&item.Answered,
		&item.Deleted,
		&item.HasAttachments,
		&item.SyncedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, apperr.NotFound("imap message not found")
		}
		return Message{}, fmt.Errorf("get imap message by uid: %w", err)
	}
	item.MessageID = optionalString(messageID)
	item.FromName = optionalString(fromName)
	item.FromAddress = optionalString(fromAddress)
	item.SentAt = optionalTime(sentAt)
	item.ReceivedAt = optionalTime(receivedAt)
	item.Snippet = optionalString(snippet)
	return item, nil
}

func (r *Repository) GetMessageLeadLinkByUser(ctx context.Context, userID, accountID uuid.UUID, uid int64) (*MessageLeadLink, error) {
	const query = `
		SELECT l.id, l.organization_id, l.account_id, l.message_uid, l.lead_id, l.created_by, l.created_at, l.updated_at
		FROM RAC_user_imap_message_leads l
		JOIN RAC_user_imap_accounts a ON a.id = l.account_id
		WHERE a.user_id = $1 AND l.account_id = $2 AND l.message_uid = $3`

	row := r.pool.QueryRow(ctx, query, userID, accountID, uid)
	var item MessageLeadLink
	if err := row.Scan(&item.ID, &item.OrganizationID, &item.AccountID, &item.MessageUID, &item.LeadID, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get imap message lead link: %w", err)
	}
	return &item, nil
}

func (r *Repository) UpsertMessageLeadLink(ctx context.Context, organizationID, createdBy, accountID, leadID uuid.UUID, uid int64) error {
	const query = `
		INSERT INTO RAC_user_imap_message_leads (
			organization_id, account_id, message_uid, lead_id, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (account_id, message_uid)
		DO UPDATE SET
			organization_id = EXCLUDED.organization_id,
			lead_id = EXCLUDED.lead_id,
			created_by = EXCLUDED.created_by,
			updated_at = now()`

	if _, err := r.pool.Exec(ctx, query, organizationID, accountID, uid, leadID, createdBy); err != nil {
		return fmt.Errorf("upsert imap message lead link: %w", err)
	}
	return nil
}

func (r *Repository) DeleteMessageLeadLinkByUser(ctx context.Context, userID, accountID uuid.UUID, uid int64) error {
	const query = `
		DELETE FROM RAC_user_imap_message_leads l
		USING RAC_user_imap_accounts a
		WHERE a.id = l.account_id
		  AND a.user_id = $1
		  AND l.account_id = $2
		  AND l.message_uid = $3`

	if _, err := r.pool.Exec(ctx, query, userID, accountID, uid); err != nil {
		return fmt.Errorf("delete imap message lead link: %w", err)
	}
	return nil
}

// GetMaxUID returns the highest UID currently synced for a given account and folder.
func (r *Repository) GetMaxUID(ctx context.Context, accountID uuid.UUID, folderName string) (int64, error) {
	maxUID, err := r.queries.GetImapMaxUID(ctx, imapdb.GetImapMaxUIDParams{
		AccountID:  toPgUUID(accountID),
		FolderName: folderName,
	})
	if err != nil {
		return 0, fmt.Errorf("get max imap uid: %w", err)
	}
	return maxUID, nil
}

func (r *Repository) ListAccountsNeedingSync(ctx context.Context, maxAge time.Duration, limit int) ([]Account, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.queries.ListImapAccountsNeedingSync(ctx, imapdb.ListImapAccountsNeedingSyncParams{
		SyncAge:    toPgInterval(maxAge),
		LimitCount: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts needing sync: %w", err)
	}

	items := make([]Account, 0)
	for _, row := range rows {
		items = append(items, accountFromModel(row))
	}
	return items, nil
}
