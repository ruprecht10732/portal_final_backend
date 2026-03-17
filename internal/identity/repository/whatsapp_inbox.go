package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/phone"
)

type WhatsAppConversation struct {
	ID                   uuid.UUID
	OrganizationID       uuid.UUID
	LeadID               *uuid.UUID
	PhoneNumber          string
	DisplayName          string
	LastMessagePreview   string
	LastMessageAt        time.Time
	LastMessageDirection string
	LastMessageStatus    string
	UnreadCount          int
	ArchivedAt           *time.Time
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type WhatsAppMessage struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	ConversationID    uuid.UUID
	LeadID            *uuid.UUID
	ExternalMessageID *string
	Direction         string
	Status            string
	PhoneNumber       string
	Body              string
	Metadata          json.RawMessage
	SentAt            *time.Time
	ReadAt            *time.Time
	FailedAt          *time.Time
	CreatedAt         time.Time
}

type WhatsAppReadSyncTarget struct {
	PhoneNumber       string
	ExternalMessageID string
}

type WhatsAppReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type WhatsAppMessageMutationParams struct {
	OrganizationID          uuid.UUID
	EventType               string
	TargetExternalMessageID string
	PhoneNumber             string
	ActorJID                string
	ActorName               string
	EventMessageID          *string
	Body                    *string
	Reaction                *string
	Metadata                json.RawMessage
	OccurredAt              *time.Time
	IsFromMe                *bool
}

type WhatsAppOutgoingMessageParams struct {
	OrganizationID    uuid.UUID
	LeadID            *uuid.UUID
	PhoneNumber       string
	Body              string
	ExternalMessageID *string
	Metadata          json.RawMessage
	SentAt            *time.Time
}

type WhatsAppIncomingMessageParams struct {
	OrganizationID    uuid.UUID
	PhoneNumber       string
	DisplayName       string
	ExternalMessageID *string
	Body              string
	Metadata          json.RawMessage
	ReceivedAt        *time.Time
}

type incomingMessageInsertParams struct {
	organizationID    uuid.UUID
	conversationID    uuid.UUID
	leadID            *uuid.UUID
	externalMessageID *string
	phoneNumber       string
	body              string
	metadata          json.RawMessage
	receivedAt        time.Time
}

type incomingConversationUpdateParams struct {
	organizationID uuid.UUID
	conversationID uuid.UUID
	leadID         *uuid.UUID
	displayName    string
	body           string
	receivedAt     time.Time
}

type outgoingConversationUpdateParams struct {
	organizationID uuid.UUID
	conversationID uuid.UUID
	leadID         *uuid.UUID
	displayName    string
	body           string
	sentAt         time.Time
}

func (r *Repository) ListWhatsAppConversations(ctx context.Context, organizationID uuid.UUID, limit, offset int) ([]WhatsAppConversation, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		SELECT id, organization_id, lead_id, phone_number, display_name,
		       last_message_preview, last_message_at, last_message_direction,
		       last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1
		  AND deleted_at IS NULL
		ORDER BY last_message_at DESC, updated_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, organizationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WhatsAppConversation, 0, limit)
	for rows.Next() {
		item, scanErr := scanWhatsAppConversation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) GetWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID) (WhatsAppConversation, error) {
	const query = `
		SELECT id, organization_id, lead_id, phone_number, display_name,
		       last_message_preview, last_message_at, last_message_direction,
		       last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := r.pool.QueryRow(ctx, query, organizationID, conversationID)
	item, err := scanWhatsAppConversationRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, err
	}
	return item, nil
}

func (r *Repository) CountWhatsAppMessages(ctx context.Context, organizationID, conversationID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM RAC_whatsapp_messages
		WHERE organization_id = $1 AND conversation_id = $2`

	var total int
	if err := r.pool.QueryRow(ctx, query, organizationID, conversationID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *Repository) ListWhatsAppMessages(ctx context.Context, organizationID, conversationID uuid.UUID, limit int) ([]WhatsAppMessage, error) {
	return r.ListWhatsAppMessagesPage(ctx, organizationID, conversationID, limit, 0)
}

func (r *Repository) ListWhatsAppMessagesPage(ctx context.Context, organizationID, conversationID uuid.UUID, limit, offset int) ([]WhatsAppMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		SELECT id, organization_id, conversation_id, lead_id, external_message_id,
		       direction, status, phone_number, body, metadata,
		       sent_at, read_at, failed_at, created_at
		FROM (
			SELECT id, organization_id, conversation_id, lead_id, external_message_id,
			       direction, status, phone_number, body, metadata,
			       sent_at, read_at, failed_at, created_at
			FROM RAC_whatsapp_messages
			WHERE organization_id = $1 AND conversation_id = $2
			ORDER BY created_at DESC, id DESC
			LIMIT $3 OFFSET $4
		) AS recent
		ORDER BY created_at ASC, id ASC`

	rows, err := r.pool.Query(ctx, query, organizationID, conversationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WhatsAppMessage, 0, limit)
	for rows.Next() {
		item, scanErr := scanWhatsAppMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) GetWhatsAppConversationByChatJID(ctx context.Context, organizationID uuid.UUID, chatJID string) (WhatsAppConversation, error) {
	trimmedChatJID := strings.TrimSpace(chatJID)
	if trimmedChatJID == "" {
		return WhatsAppConversation{}, ErrNotFound
	}

	const byMessageChatIDQuery = `
		SELECT id, organization_id, lead_id, phone_number, display_name,
		       last_message_preview, last_message_at, last_message_direction,
		       last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1
		  AND deleted_at IS NULL
		  AND id = (
			SELECT conversation_id
			FROM RAC_whatsapp_messages
			WHERE organization_id = $1
			  AND metadata IS NOT NULL
			  AND TRIM(metadata->'payload'->>'chat_id') = $2
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		  )`

	conversation, err := scanWhatsAppConversationRow(r.pool.QueryRow(ctx, byMessageChatIDQuery, organizationID, trimmedChatJID))
	if err == nil {
		return conversation, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, err
	}

	normalizedPhone := normalizeWhatsAppPhoneNumber(trimmedChatJID)
	if normalizedPhone == "" {
		return WhatsAppConversation{}, ErrNotFound
	}

	const byPhoneQuery = `
		SELECT id, organization_id, lead_id, phone_number, display_name,
		       last_message_preview, last_message_at, last_message_direction,
		       last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1
		  AND phone_number = $2
		  AND deleted_at IS NULL
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1`

	conversation, err = scanWhatsAppConversationRow(r.pool.QueryRow(ctx, byPhoneQuery, organizationID, normalizedPhone))
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, err
	}
	return conversation, nil
}

func (r *Repository) GetLatestUnreadWhatsAppReadSyncTarget(ctx context.Context, organizationID, conversationID uuid.UUID) (*WhatsAppReadSyncTarget, error) {
	const query = `
		SELECT messages.phone_number, messages.external_message_id
		FROM RAC_whatsapp_messages messages
		JOIN RAC_whatsapp_conversations conversations
		  ON conversations.organization_id = messages.organization_id
		 AND conversations.id = messages.conversation_id
		WHERE messages.organization_id = $1
		  AND messages.conversation_id = $2
		  AND messages.direction = 'inbound'
		  AND messages.read_at IS NULL
		  AND messages.external_message_id IS NOT NULL
		  AND conversations.deleted_at IS NULL
		ORDER BY messages.created_at DESC
		LIMIT 1`

	var target WhatsAppReadSyncTarget
	err := r.pool.QueryRow(ctx, query, organizationID, conversationID).Scan(&target.PhoneNumber, &target.ExternalMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) MarkWhatsAppConversationRead(ctx context.Context, organizationID, conversationID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const updateConversation = `
		UPDATE RAC_whatsapp_conversations
		SET unread_count = 0, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`
	result, err := tx.Exec(ctx, updateConversation, organizationID, conversationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	const updateMessages = `
		UPDATE RAC_whatsapp_messages
		SET read_at = now()
		WHERE organization_id = $1
		  AND conversation_id = $2
		  AND direction = 'inbound'
		  AND read_at IS NULL`
	if _, err = tx.Exec(ctx, updateMessages, organizationID, conversationID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) SetWhatsAppConversationArchived(ctx context.Context, organizationID, conversationID uuid.UUID, value bool) (WhatsAppConversation, error) {
	const query = `
		UPDATE RAC_whatsapp_conversations
		SET archived_at = CASE WHEN $3 THEN now() ELSE NULL END,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	conversation, err := scanWhatsAppConversationRow(r.pool.QueryRow(ctx, query, organizationID, conversationID, value))
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, err
	}
	return conversation, nil
}

func (r *Repository) DeleteWhatsAppConversation(ctx context.Context, organizationID, conversationID uuid.UUID) (WhatsAppConversation, error) {
	const query = `
		UPDATE RAC_whatsapp_conversations
		SET deleted_at = now(),
		    unread_count = 0,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	conversation, err := scanWhatsAppConversationRow(r.pool.QueryRow(ctx, query, organizationID, conversationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, err
	}
	return conversation, nil
}

func (r *Repository) UpdateWhatsAppConversationLead(ctx context.Context, organizationID, conversationID uuid.UUID, leadID *uuid.UUID) (WhatsAppConversation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppConversation{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const updateConversation = `
		UPDATE RAC_whatsapp_conversations
		SET lead_id = $3,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	conversation, err := scanWhatsAppConversationRow(tx.QueryRow(ctx, updateConversation, organizationID, conversationID, toNullableUUID(leadID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, err
	}

	const updateMessages = `
		UPDATE RAC_whatsapp_messages
		SET lead_id = $3
		WHERE organization_id = $1 AND conversation_id = $2`
	if _, err = tx.Exec(ctx, updateMessages, organizationID, conversationID, toNullableUUID(leadID)); err != nil {
		return WhatsAppConversation{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return WhatsAppConversation{}, err
	}

	return conversation, nil
}

func (r *Repository) RecordSentWhatsAppMessage(ctx context.Context, params WhatsAppOutgoingMessageParams) (WhatsAppConversation, WhatsAppMessage, error) {
	conversation, message, _, err := r.recordSentWhatsAppMessage(ctx, params)
	return conversation, message, err
}

func (r *Repository) SyncSentWhatsAppMessage(ctx context.Context, params WhatsAppOutgoingMessageParams) (WhatsAppConversation, WhatsAppMessage, bool, error) {
	return r.recordSentWhatsAppMessage(ctx, params)
}

func (r *Repository) recordSentWhatsAppMessage(ctx context.Context, params WhatsAppOutgoingMessageParams) (WhatsAppConversation, WhatsAppMessage, bool, error) {
	phoneNumber, body, metadata, sentAt, err := prepareOutgoingWhatsAppMessage(params)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	leadID, displayName, err := r.resolveConversationLead(ctx, tx, params.OrganizationID, params.LeadID, phoneNumber)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	conversation, err := r.upsertOutgoingConversation(ctx, tx, params.OrganizationID, leadID, phoneNumber, displayName)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	message, created, err := r.insertOutgoingWhatsAppMessage(ctx, tx, outgoingMessageInsertParams{
		organizationID:    params.OrganizationID,
		conversationID:    conversation.ID,
		leadID:            leadID,
		externalMessageID: params.ExternalMessageID,
		phoneNumber:       phoneNumber,
		body:              body,
		metadata:          metadata,
		sentAt:            sentAt,
	})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	if !created {
		message, conversation, err = r.loadDuplicateOutgoingMessage(ctx, tx, params.OrganizationID, params.ExternalMessageID)
		if err != nil {
			return WhatsAppConversation{}, WhatsAppMessage{}, false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return WhatsAppConversation{}, WhatsAppMessage{}, false, err
		}
		return conversation, message, false, nil
	}

	conversation, err = r.finalizeOutgoingConversation(ctx, tx, outgoingConversationUpdateParams{
		organizationID: params.OrganizationID,
		conversationID: conversation.ID,
		leadID:         leadID,
		displayName:    displayName,
		body:           body,
		sentAt:         sentAt,
	})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	return conversation, message, true, nil
}

func prepareOutgoingWhatsAppMessage(params WhatsAppOutgoingMessageParams) (string, string, json.RawMessage, time.Time, error) {
	phoneNumber := strings.TrimSpace(phone.NormalizeE164(params.PhoneNumber))
	if phoneNumber == "" {
		return "", "", nil, time.Time{}, fmt.Errorf("phone number is required")
	}
	body := strings.TrimSpace(params.Body)
	if body == "" {
		return "", "", nil, time.Time{}, fmt.Errorf("message body is required")
	}
	metadata := params.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	sentAt := time.Now().UTC()
	if params.SentAt != nil {
		sentAt = params.SentAt.UTC()
	}
	return phoneNumber, body, metadata, sentAt, nil
}

type outgoingMessageInsertParams struct {
	organizationID    uuid.UUID
	conversationID    uuid.UUID
	leadID            *uuid.UUID
	externalMessageID *string
	phoneNumber       string
	body              string
	metadata          json.RawMessage
	sentAt            time.Time
}

func (r *Repository) upsertOutgoingConversation(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, displayName string) (WhatsAppConversation, error) {
	return r.upsertWhatsAppConversation(ctx, tx, organizationID, leadID, phoneNumber, displayName)
}

func (r *Repository) upsertWhatsAppConversation(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, displayName string) (WhatsAppConversation, error) {
	const query = `
		INSERT INTO RAC_whatsapp_conversations (
			organization_id, lead_id, phone_number, display_name,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (organization_id, phone_number)
		DO UPDATE SET
			lead_id = COALESCE(EXCLUDED.lead_id, RAC_whatsapp_conversations.lead_id),
			display_name = CASE
				WHEN EXCLUDED.display_name <> '' THEN EXCLUDED.display_name
				ELSE RAC_whatsapp_conversations.display_name
			END,
			deleted_at = NULL,
			updated_at = now()
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	return scanWhatsAppConversationRow(tx.QueryRow(ctx, query, organizationID, toNullableUUID(leadID), phoneNumber, displayName))
}

func (r *Repository) insertOutgoingWhatsAppMessage(
	ctx context.Context,
	tx pgx.Tx,
	params outgoingMessageInsertParams,
) (WhatsAppMessage, bool, error) {
	const query = `
		INSERT INTO RAC_whatsapp_messages (
			organization_id, conversation_id, lead_id, external_message_id, direction, status,
			phone_number, body, metadata, sent_at, created_at
		) VALUES ($1, $2, $3, $4, 'outbound', 'sent', $5, $6, $7, $8, $8)
		ON CONFLICT (organization_id, external_message_id) WHERE external_message_id IS NOT NULL
		DO NOTHING
		RETURNING id, organization_id, conversation_id, lead_id, external_message_id,
		          direction, status, phone_number, body, metadata,
		          sent_at, read_at, failed_at, created_at`

	message, err := scanWhatsAppMessageRow(tx.QueryRow(
		ctx,
		query,
		params.organizationID,
		params.conversationID,
		toNullableUUID(params.leadID),
		params.externalMessageID,
		params.phoneNumber,
		params.body,
		params.metadata,
		params.sentAt,
	))
	if err == nil {
		return message, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppMessage{}, false, nil
	}
	return WhatsAppMessage{}, false, err
}

func (r *Repository) loadDuplicateOutgoingMessage(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, externalMessageID *string) (WhatsAppMessage, WhatsAppConversation, error) {
	if externalMessageID == nil || strings.TrimSpace(*externalMessageID) == "" {
		return WhatsAppMessage{}, WhatsAppConversation{}, fmt.Errorf("failed to persist outbound message")
	}
	return r.getWhatsAppMessageByExternalID(ctx, tx, organizationID, *externalMessageID)
}

func (r *Repository) finalizeOutgoingConversation(ctx context.Context, tx pgx.Tx, params outgoingConversationUpdateParams) (WhatsAppConversation, error) {
	const query = `
		UPDATE RAC_whatsapp_conversations
		SET lead_id = COALESCE($3, lead_id),
		    display_name = CASE WHEN $4 <> '' THEN $4 ELSE display_name END,
		    last_message_preview = $5,
		    last_message_at = $6,
		    last_message_direction = 'outbound',
		    last_message_status = 'sent',
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	return scanWhatsAppConversationRow(tx.QueryRow(
		ctx,
		query,
		params.organizationID,
		params.conversationID,
		toNullableUUID(params.leadID),
		params.displayName,
		truncateWhatsAppPreview(params.body),
		params.sentAt,
	))
}

func (r *Repository) RecordIncomingWhatsAppMessage(ctx context.Context, params WhatsAppIncomingMessageParams) (WhatsAppConversation, WhatsAppMessage, bool, error) {
	phoneNumber, body, metadata, receivedAt, err := prepareIncomingWhatsAppMessage(params)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	leadID, resolvedDisplayName, err := r.resolveConversationLead(ctx, tx, params.OrganizationID, nil, phoneNumber)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = resolvedDisplayName
	}

	conversation, err := r.upsertIncomingConversation(ctx, tx, params.OrganizationID, leadID, phoneNumber, displayName)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	message, created, err := r.insertIncomingWhatsAppMessage(ctx, tx, incomingMessageInsertParams{
		organizationID:    params.OrganizationID,
		conversationID:    conversation.ID,
		leadID:            leadID,
		externalMessageID: params.ExternalMessageID,
		phoneNumber:       phoneNumber,
		body:              body,
		metadata:          metadata,
		receivedAt:        receivedAt,
	})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	if !created {
		message, conversation, err = r.loadDuplicateIncomingMessage(ctx, tx, params.OrganizationID, params.ExternalMessageID)
		if err != nil {
			return WhatsAppConversation{}, WhatsAppMessage{}, false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return WhatsAppConversation{}, WhatsAppMessage{}, false, err
		}
		return conversation, message, false, nil
	}

	conversation, err = r.finalizeIncomingConversation(ctx, tx, incomingConversationUpdateParams{
		organizationID: params.OrganizationID,
		conversationID: conversation.ID,
		leadID:         leadID,
		displayName:    displayName,
		body:           body,
		receivedAt:     receivedAt,
	})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	return conversation, message, true, nil
}

func prepareIncomingWhatsAppMessage(params WhatsAppIncomingMessageParams) (string, string, json.RawMessage, time.Time, error) {
	phoneNumber := normalizeWhatsAppPhoneNumber(params.PhoneNumber)
	if phoneNumber == "" {
		return "", "", nil, time.Time{}, fmt.Errorf("phone number is required")
	}
	body := strings.TrimSpace(params.Body)
	if body == "" {
		return "", "", nil, time.Time{}, fmt.Errorf("message body is required")
	}
	metadata := params.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	receivedAt := time.Now().UTC()
	if params.ReceivedAt != nil {
		receivedAt = params.ReceivedAt.UTC()
	}
	return phoneNumber, body, metadata, receivedAt, nil
}

func (r *Repository) upsertIncomingConversation(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, displayName string) (WhatsAppConversation, error) {
	return r.upsertWhatsAppConversation(ctx, tx, organizationID, leadID, phoneNumber, displayName)
}

func (r *Repository) insertIncomingWhatsAppMessage(
	ctx context.Context,
	tx pgx.Tx,
	params incomingMessageInsertParams,
) (WhatsAppMessage, bool, error) {
	const query = `
		INSERT INTO RAC_whatsapp_messages (
			organization_id, conversation_id, lead_id, external_message_id,
			direction, status, phone_number, body, metadata, created_at
		) VALUES ($1, $2, $3, $4, 'inbound', 'received', $5, $6, $7, $8)
		ON CONFLICT (organization_id, external_message_id) WHERE external_message_id IS NOT NULL
		DO NOTHING
		RETURNING id, organization_id, conversation_id, lead_id, external_message_id,
		          direction, status, phone_number, body, metadata,
		          sent_at, read_at, failed_at, created_at`

	message, err := scanWhatsAppMessageRow(tx.QueryRow(
		ctx,
		query,
		params.organizationID,
		params.conversationID,
		toNullableUUID(params.leadID),
		params.externalMessageID,
		params.phoneNumber,
		params.body,
		params.metadata,
		params.receivedAt,
	))
	if err == nil {
		return message, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppMessage{}, false, nil
	}
	return WhatsAppMessage{}, false, err
}

func (r *Repository) loadDuplicateIncomingMessage(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, externalMessageID *string) (WhatsAppMessage, WhatsAppConversation, error) {
	if externalMessageID == nil || strings.TrimSpace(*externalMessageID) == "" {
		return WhatsAppMessage{}, WhatsAppConversation{}, fmt.Errorf("failed to persist inbound message")
	}
	return r.getWhatsAppMessageByExternalID(ctx, tx, organizationID, *externalMessageID)
}

func (r *Repository) finalizeIncomingConversation(ctx context.Context, tx pgx.Tx, params incomingConversationUpdateParams) (WhatsAppConversation, error) {
	const query = `
		UPDATE RAC_whatsapp_conversations
		SET lead_id = COALESCE($3, lead_id),
		    display_name = CASE WHEN $4 <> '' THEN $4 ELSE display_name END,
		    last_message_preview = $5,
		    last_message_at = $6,
		    last_message_direction = 'inbound',
		    last_message_status = 'received',
		    unread_count = unread_count + 1,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	return scanWhatsAppConversationRow(tx.QueryRow(
		ctx,
		query,
		params.organizationID,
		params.conversationID,
		toNullableUUID(params.leadID),
		params.displayName,
		truncateWhatsAppPreview(params.body),
		params.receivedAt,
	))
}

func (r *Repository) CountUnreadWhatsAppConversations(ctx context.Context, organizationID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1
		  AND deleted_at IS NULL
		  AND archived_at IS NULL
		  AND unread_count > 0`

	var count int
	if err := r.pool.QueryRow(ctx, query, organizationID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) ListRecentWhatsAppReplyExamples(ctx context.Context, organizationID uuid.UUID, referenceLeadID *uuid.UUID, excludeConversationID uuid.UUID, limit int) ([]WhatsAppReplyExample, error) {
	if limit <= 0 {
		limit = 4
	}

	const query = `
		WITH reference_service AS (
			SELECT
				st.name AS service_type,
				ls.pipeline_stage::text AS pipeline_stage
			FROM RAC_lead_services ls
			JOIN RAC_service_types st
			  ON st.id = ls.service_type_id
			 AND st.organization_id = ls.organization_id
			WHERE ls.organization_id = $1
			  AND ls.lead_id = $2
			ORDER BY
				CASE WHEN ls.pipeline_stage::text NOT IN ('Completed', 'Lost') THEN 0 ELSE 1 END,
				ls.updated_at DESC,
				ls.created_at DESC
			LIMIT 1
		),
		WITH ranked_examples AS (
			SELECT
				outgoing.conversation_id,
				outgoing.body AS reply,
				outgoing.created_at,
				(
					SELECT inbound.body
					FROM RAC_whatsapp_messages inbound
					WHERE inbound.organization_id = outgoing.organization_id
					  AND inbound.conversation_id = outgoing.conversation_id
					  AND inbound.direction = 'inbound'
					  AND inbound.created_at <= outgoing.created_at
					  AND NULLIF(BTRIM(inbound.body), '') IS NOT NULL
					ORDER BY inbound.created_at DESC
					LIMIT 1
				) AS customer_message,
				svc.service_type,
				svc.pipeline_stage,
				ROW_NUMBER() OVER (
					PARTITION BY outgoing.conversation_id
					ORDER BY outgoing.created_at DESC
				) AS conversation_rank
			FROM RAC_whatsapp_messages outgoing
			JOIN RAC_whatsapp_conversations conversations
			  ON conversations.organization_id = outgoing.organization_id
			 AND conversations.id = outgoing.conversation_id
			 AND conversations.deleted_at IS NULL
			LEFT JOIN LATERAL (
				SELECT
					st.name AS service_type,
					ls.pipeline_stage::text AS pipeline_stage
				FROM RAC_lead_services ls
				JOIN RAC_service_types st
				  ON st.id = ls.service_type_id
				 AND st.organization_id = ls.organization_id
				WHERE conversations.lead_id IS NOT NULL
				  AND ls.organization_id = conversations.organization_id
				  AND ls.lead_id = conversations.lead_id
				ORDER BY
					CASE WHEN ls.pipeline_stage::text NOT IN ('Completed', 'Lost') THEN 0 ELSE 1 END,
					ls.updated_at DESC,
					ls.created_at DESC
				LIMIT 1
			) svc ON TRUE
			WHERE outgoing.organization_id = $1
			  AND outgoing.conversation_id <> $3
			  AND outgoing.direction = 'outbound'
			  AND outgoing.status IN ('sent', 'delivered', 'read')
			  AND NULLIF(BTRIM(outgoing.body), '') IS NOT NULL
		),
		scored_examples AS (
			SELECT
				ranked_examples.customer_message,
				ranked_examples.reply,
				ranked_examples.created_at,
				CASE
					WHEN reference_service.service_type IS NOT NULL AND ranked_examples.service_type = reference_service.service_type THEN 0
					WHEN reference_service.service_type IS NOT NULL THEN 1
					ELSE 2
				END AS service_rank,
				CASE
					WHEN reference_service.pipeline_stage IS NOT NULL AND ranked_examples.pipeline_stage = reference_service.pipeline_stage THEN 0
					WHEN reference_service.pipeline_stage IS NOT NULL THEN 1
					ELSE 2
				END AS stage_rank
			FROM ranked_examples
			LEFT JOIN reference_service ON TRUE
			WHERE ranked_examples.conversation_rank = 1
			  AND ranked_examples.customer_message IS NOT NULL
		)
		SELECT customer_message, reply, created_at
		FROM scored_examples
		ORDER BY service_rank ASC, stage_rank ASC, created_at DESC
		LIMIT $4`

	rows, err := r.pool.Query(ctx, query, organizationID, toNullableUUID(referenceLeadID), excludeConversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	examples := make([]WhatsAppReplyExample, 0, limit)
	for rows.Next() {
		var item WhatsAppReplyExample
		if err := rows.Scan(&item.CustomerMessage, &item.Reply, &item.CreatedAt); err != nil {
			return nil, err
		}
		examples = append(examples, item)
	}

	return examples, rows.Err()
}

func (r *Repository) GetWhatsAppMessageByExternalID(ctx context.Context, organizationID uuid.UUID, externalMessageID string) (WhatsAppMessage, WhatsAppConversation, error) {
	targetID := strings.TrimSpace(externalMessageID)
	if targetID == "" {
		return WhatsAppMessage{}, WhatsAppConversation{}, ErrNotFound
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppMessage{}, WhatsAppConversation{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	message, conversation, err := r.getWhatsAppMessageByExternalID(ctx, tx, organizationID, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppMessage{}, WhatsAppConversation{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppMessage{}, WhatsAppConversation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return WhatsAppMessage{}, WhatsAppConversation{}, err
	}
	return message, conversation, nil
}

func (r *Repository) UpdateWhatsAppMessageByExternalID(ctx context.Context, organizationID uuid.UUID, externalMessageID string, body string, metadata json.RawMessage) (WhatsAppConversation, WhatsAppMessage, error) {
	targetID := strings.TrimSpace(externalMessageID)
	if targetID == "" {
		return WhatsAppConversation{}, WhatsAppMessage{}, ErrNotFound
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	message, conversation, err := r.getWhatsAppMessageByExternalID(ctx, tx, organizationID, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, WhatsAppMessage{}, ErrNotFound
	}
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, err
	}

	message, err = r.updateWhatsAppMessageMutation(ctx, tx, message.ID, strings.TrimSpace(body), metadata)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, err
	}
	conversation, err = r.updateConversationForMessageMutation(ctx, tx, conversation, message)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, err
	}
	return conversation, message, nil
}

func (r *Repository) ApplyWhatsAppMessageReceipt(ctx context.Context, organizationID uuid.UUID, externalMessageIDs []string, status string, receiptAt *time.Time) ([]WhatsAppConversation, []WhatsAppMessage, error) {
	ids := normalizeReceiptIDs(externalMessageIDs)
	if len(ids) == 0 {
		return nil, nil, nil
	}
	resolvedStatus, ok := resolveReceiptStatus(status)
	if !ok {
		return nil, nil, nil
	}
	receiptTime := time.Now().UTC()
	if receiptAt != nil {
		receiptTime = receiptAt.UTC()
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	messages, err := r.updateOutboundMessageStatuses(ctx, tx, organizationID, ids, resolvedStatus, receiptTime)
	if err != nil {
		return nil, nil, err
	}
	if len(messages) == 0 {
		if err = tx.Commit(ctx); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	}

	conversationIDs := uniqueConversationIDs(messages)
	conversations, err := r.updateReceiptConversationStatuses(ctx, tx, organizationID, ids, conversationIDs, resolvedStatus)
	if err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return conversations, messages, nil
}

func (r *Repository) ApplyWhatsAppMessageMutation(ctx context.Context, params WhatsAppMessageMutationParams) (WhatsAppConversation, WhatsAppMessage, bool, error) {
	targetID := strings.TrimSpace(params.TargetExternalMessageID)
	if targetID == "" {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	message, conversation, err := r.getWhatsAppMessageByExternalID(ctx, tx, params.OrganizationID, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, nil
	}
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	updatedBody, updatedMetadata, err := applyWhatsAppMessageMutationData(message, params)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	message, err = r.updateWhatsAppMessageMutation(ctx, tx, message.ID, updatedBody, updatedMetadata)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	conversation, err = r.updateConversationForMessageMutation(ctx, tx, conversation, message)
	if err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return WhatsAppConversation{}, WhatsAppMessage{}, false, err
	}

	return conversation, message, true, nil
}

type leadLookup struct {
	ID        uuid.UUID
	FirstName string
	LastName  string
}

func (r *Repository) resolveConversationLead(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, requestedLeadID *uuid.UUID, phoneNumber string) (*uuid.UUID, string, error) {
	if requestedLeadID != nil {
		const query = `
			SELECT id, consumer_first_name, consumer_last_name
			FROM RAC_leads
			WHERE organization_id = $1 AND id = $2`
		row := tx.QueryRow(ctx, query, organizationID, *requestedLeadID)
		lead, err := scanLeadLookupRow(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		if err != nil {
			return nil, "", err
		}
		return &lead.ID, strings.TrimSpace(lead.FirstName + " " + lead.LastName), nil
	}

	const query = `
		SELECT id, consumer_first_name, consumer_last_name
		FROM RAC_leads
		WHERE organization_id = $1 AND consumer_phone = $2
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1`
	row := tx.QueryRow(ctx, query, organizationID, phoneNumber)
	lead, err := scanLeadLookupRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &lead.ID, strings.TrimSpace(lead.FirstName + " " + lead.LastName), nil
}

func truncateWhatsAppPreview(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 160 {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:157]) + "..."
}

func normalizeWhatsAppPhoneNumber(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		trimmed = trimmed[:at]
	}
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "+") {
		trimmed = "+" + trimmed
	}
	return strings.TrimSpace(phone.NormalizeE164(trimmed))
}

func normalizeReceiptIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func resolveReceiptStatus(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "delivered":
		return "delivered", true
	case "read":
		return "read", true
	default:
		return "", false
	}
}

func applyWhatsAppMessageMutationData(message WhatsAppMessage, params WhatsAppMessageMutationParams) (string, json.RawMessage, error) {
	updatedBody := message.Body
	if params.Body != nil {
		updatedBody = strings.TrimSpace(*params.Body)
	}

	metadataMap := make(map[string]any)
	if len(message.Metadata) > 0 {
		if err := json.Unmarshal(message.Metadata, &metadataMap); err != nil {
			metadataMap = make(map[string]any)
		}
	}

	portalData, _ := metadataMap["portal"].(map[string]any)
	if portalData == nil {
		portalData = make(map[string]any)
	}
	if _, exists := portalData["originalBody"]; !exists && strings.TrimSpace(message.Body) != "" && strings.TrimSpace(message.Body) != updatedBody {
		portalData["originalBody"] = message.Body
	}

	switch params.EventType {
	case "message.edited":
		portalData["edited"] = buildMutationAuditPayload(params)
	case "message.deleted":
		portalData["deleted"] = buildMutationAuditPayload(params)
	case "message.revoked":
		portalData["revoked"] = buildMutationAuditPayload(params)
	case "message.reaction":
		portalData["reactions"] = mergeReactionMetadata(portalData["reactions"], params)
	}

	metadataMap["portal"] = portalData
	metadataMap["lastMutationEvent"] = params.EventType
	if len(params.Metadata) > 0 {
		var mutationPayload any
		if err := json.Unmarshal(params.Metadata, &mutationPayload); err == nil {
			metadataMap["lastMutationPayload"] = mutationPayload
		}
	}

	encoded, err := json.Marshal(metadataMap)
	if err != nil {
		return "", nil, err
	}
	return updatedBody, encoded, nil
}

func buildMutationAuditPayload(params WhatsAppMessageMutationParams) map[string]any {
	payload := map[string]any{}
	if params.ActorJID != "" {
		payload["actorJid"] = params.ActorJID
	}
	if params.ActorName != "" {
		payload["actorName"] = params.ActorName
	}
	if params.EventMessageID != nil {
		payload["eventMessageId"] = *params.EventMessageID
	}
	if params.OccurredAt != nil {
		payload["occurredAt"] = params.OccurredAt.Format(time.RFC3339)
	}
	if params.IsFromMe != nil {
		payload["isFromMe"] = *params.IsFromMe
	}
	return payload
}

func mergeReactionMetadata(current any, params WhatsAppMessageMutationParams) []map[string]any {
	reactions := make([]map[string]any, 0)
	switch typed := current.(type) {
	case []any:
		for _, entry := range typed {
			reaction, ok := entry.(map[string]any)
			if ok {
				reactions = append(reactions, reaction)
			}
		}
	case []map[string]any:
		reactions = append(reactions, typed...)
	}

	actorKey := strings.TrimSpace(params.ActorJID)
	filtered := reactions[:0]
	for _, reaction := range reactions {
		if actor, _ := reaction["actorJid"].(string); actor == actorKey && actorKey != "" {
			continue
		}
		filtered = append(filtered, reaction)
	}
	reactions = filtered

	if params.Reaction != nil && strings.TrimSpace(*params.Reaction) != "" {
		reactionEntry := buildMutationAuditPayload(params)
		reactionEntry["reaction"] = strings.TrimSpace(*params.Reaction)
		reactions = append(reactions, reactionEntry)
	}

	return reactions
}

func (r *Repository) updateWhatsAppMessageMutation(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, body string, metadata json.RawMessage) (WhatsAppMessage, error) {
	const query = `
		UPDATE RAC_whatsapp_messages
		SET body = $2,
		    metadata = $3
		WHERE id = $1
		RETURNING id, organization_id, conversation_id, lead_id, external_message_id,
		          direction, status, phone_number, body, metadata,
		          sent_at, read_at, failed_at, created_at`

	return scanWhatsAppMessageRow(tx.QueryRow(ctx, query, messageID, body, metadata))
}

func (r *Repository) updateConversationForMessageMutation(ctx context.Context, tx pgx.Tx, conversation WhatsAppConversation, message WhatsAppMessage) (WhatsAppConversation, error) {
	const latestQuery = `
		SELECT NOT EXISTS (
			SELECT 1
			FROM RAC_whatsapp_messages newer
			WHERE newer.organization_id = $1
			  AND newer.conversation_id = $2
			  AND (
			    newer.created_at > $3
			    OR (newer.created_at = $3 AND newer.id <> $4)
			  )
		)`

	var isLatest bool
	if err := tx.QueryRow(ctx, latestQuery, conversation.OrganizationID, conversation.ID, message.CreatedAt, message.ID).Scan(&isLatest); err != nil {
		return WhatsAppConversation{}, err
	}
	if !isLatest {
		return conversation, nil
	}

	const updateConversation = `
		UPDATE RAC_whatsapp_conversations
		SET last_message_preview = $3,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	return scanWhatsAppConversationRow(tx.QueryRow(ctx, updateConversation, conversation.OrganizationID, conversation.ID, truncateWhatsAppPreview(message.Body)))
}

func toNullableUUID(value *uuid.UUID) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

type conversationScanner interface {
	Scan(dest ...any) error
}

func scanWhatsAppConversation(rows pgx.Rows) (WhatsAppConversation, error) {
	return scanWhatsAppConversationRow(rows)
}

func scanWhatsAppConversationRow(scanner conversationScanner) (WhatsAppConversation, error) {
	var item WhatsAppConversation
	var leadID *uuid.UUID
	err := scanner.Scan(
		&item.ID,
		&item.OrganizationID,
		&leadID,
		&item.PhoneNumber,
		&item.DisplayName,
		&item.LastMessagePreview,
		&item.LastMessageAt,
		&item.LastMessageDirection,
		&item.LastMessageStatus,
		&item.UnreadCount,
		&item.ArchivedAt,
		&item.DeletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return WhatsAppConversation{}, err
	}
	item.LeadID = leadID
	return item, nil
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanWhatsAppMessage(rows pgx.Rows) (WhatsAppMessage, error) {
	return scanWhatsAppMessageRow(rows)
}

func scanWhatsAppMessageRow(scanner messageScanner) (WhatsAppMessage, error) {
	var item WhatsAppMessage
	var leadID *uuid.UUID
	var externalMessageID *string
	err := scanner.Scan(
		&item.ID,
		&item.OrganizationID,
		&item.ConversationID,
		&leadID,
		&externalMessageID,
		&item.Direction,
		&item.Status,
		&item.PhoneNumber,
		&item.Body,
		&item.Metadata,
		&item.SentAt,
		&item.ReadAt,
		&item.FailedAt,
		&item.CreatedAt,
	)
	if err != nil {
		return WhatsAppMessage{}, err
	}
	item.LeadID = leadID
	item.ExternalMessageID = externalMessageID
	return item, nil
}

func scanLeadLookupRow(scanner interface{ Scan(dest ...any) error }) (leadLookup, error) {
	var item leadLookup
	err := scanner.Scan(&item.ID, &item.FirstName, &item.LastName)
	if err != nil {
		return leadLookup{}, err
	}
	return item, nil
}

func (r *Repository) getWhatsAppMessageByExternalID(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, externalMessageID string) (WhatsAppMessage, WhatsAppConversation, error) {
	const messageQuery = `
		SELECT id, organization_id, conversation_id, lead_id, external_message_id,
		       direction, status, phone_number, body, metadata,
		       sent_at, read_at, failed_at, created_at
		FROM RAC_whatsapp_messages
		WHERE organization_id = $1 AND external_message_id = $2`

	message, err := scanWhatsAppMessageRow(tx.QueryRow(ctx, messageQuery, organizationID, externalMessageID))
	if err != nil {
		return WhatsAppMessage{}, WhatsAppConversation{}, err
	}

	const conversationQuery = `
		SELECT id, organization_id, lead_id, phone_number, display_name,
		       last_message_preview, last_message_at, last_message_direction,
		       last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`

	conversation, err := scanWhatsAppConversationRow(tx.QueryRow(ctx, conversationQuery, organizationID, message.ConversationID))
	if err != nil {
		return WhatsAppMessage{}, WhatsAppConversation{}, err
	}

	return message, conversation, nil
}

func (r *Repository) updateOutboundMessageStatuses(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, ids []string, status string, receiptAt time.Time) ([]WhatsAppMessage, error) {
	const query = `
		UPDATE RAC_whatsapp_messages
		SET status = $3,
		    read_at = CASE
		        WHEN $3 = 'read' THEN COALESCE(read_at, $4)
		        ELSE read_at
		    END
		WHERE organization_id = $1
		  AND external_message_id = ANY($2)
		  AND direction = 'outbound'
		  AND (
		    (status = 'sent' AND $3 = 'delivered')
		    OR (status IN ('sent', 'delivered') AND $3 = 'read')
		  )
		RETURNING id, organization_id, conversation_id, lead_id, external_message_id,
		          direction, status, phone_number, body, metadata,
		          sent_at, read_at, failed_at, created_at`

	rows, err := tx.Query(ctx, query, organizationID, ids, status, receiptAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WhatsAppMessage, 0, len(ids))
	for rows.Next() {
		item, scanErr := scanWhatsAppMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func uniqueConversationIDs(messages []WhatsAppMessage) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(messages))
	ids := make([]uuid.UUID, 0, len(messages))
	for _, message := range messages {
		if _, exists := seen[message.ConversationID]; exists {
			continue
		}
		seen[message.ConversationID] = struct{}{}
		ids = append(ids, message.ConversationID)
	}
	return ids
}

func (r *Repository) updateReceiptConversationStatuses(ctx context.Context, tx pgx.Tx, organizationID uuid.UUID, externalMessageIDs []string, conversationIDs []uuid.UUID, status string) ([]WhatsAppConversation, error) {
	const query = `
		UPDATE RAC_whatsapp_conversations AS conversations
		SET last_message_status = $3,
		    updated_at = now()
		WHERE conversations.organization_id = $1
		  AND conversations.id = ANY($2)
		  AND conversations.last_message_direction = 'outbound'
		  AND EXISTS (
		    SELECT 1
		    FROM RAC_whatsapp_messages messages
		    WHERE messages.organization_id = conversations.organization_id
		      AND messages.conversation_id = conversations.id
		      AND messages.external_message_id = ANY($4)
		      AND NOT EXISTS (
		        SELECT 1
		        FROM RAC_whatsapp_messages newer
		        WHERE newer.organization_id = messages.organization_id
		          AND newer.conversation_id = messages.conversation_id
		          AND (
		            newer.created_at > messages.created_at
		            OR (newer.created_at = messages.created_at AND newer.id <> messages.id)
		          )
		      )
		  )
		RETURNING id, organization_id, lead_id, phone_number, display_name,
		          last_message_preview, last_message_at, last_message_direction,
		          last_message_status, unread_count, archived_at, deleted_at, created_at, updated_at`

	rows, err := tx.Query(ctx, query, organizationID, conversationIDs, status, externalMessageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WhatsAppConversation, 0, len(conversationIDs))
	for rows.Next() {
		item, scanErr := scanWhatsAppConversation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) LookupWhatsAppConversationChatJID(ctx context.Context, organizationID, conversationID uuid.UUID) (string, error) {
	const query = `
		SELECT metadata->'payload'->>'chat_id'
		FROM RAC_whatsapp_messages
		WHERE organization_id = $1
		  AND conversation_id = $2
		  AND metadata IS NOT NULL
		  AND metadata->'payload'->>'chat_id' IS NOT NULL
		  AND TRIM(metadata->'payload'->>'chat_id') != ''
		ORDER BY created_at DESC
		LIMIT 1`

	var chatJID string
	if err := r.pool.QueryRow(ctx, query, organizationID, conversationID).Scan(&chatJID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(chatJID), nil
}

func (r *Repository) LookupWhatsAppConversationChatJIDs(ctx context.Context, organizationID uuid.UUID, conversationIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	result := make(map[uuid.UUID]string, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return result, nil
	}

	const query = `
		SELECT DISTINCT ON (conversation_id)
			conversation_id,
			TRIM(metadata->'payload'->>'chat_id') AS chat_jid
		FROM RAC_whatsapp_messages
		WHERE organization_id = $1
		  AND conversation_id = ANY($2)
		  AND metadata IS NOT NULL
		  AND metadata->'payload'->>'chat_id' IS NOT NULL
		  AND TRIM(metadata->'payload'->>'chat_id') != ''
		ORDER BY conversation_id, created_at DESC, id DESC`

	rows, err := r.pool.Query(ctx, query, organizationID, conversationIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var conversationID uuid.UUID
		var chatJID string
		if scanErr := rows.Scan(&conversationID, &chatJID); scanErr != nil {
			return nil, scanErr
		}
		result[conversationID] = strings.TrimSpace(chatJID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

var _ = (*pgxpool.Pool)(nil)
