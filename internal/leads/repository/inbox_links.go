package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type LinkedWhatsAppConversation struct {
	ConversationID         uuid.UUID
	PhoneNumber            string
	DisplayName            string
	LastMessagePreview     string
	LastMessageAt          *time.Time
	LastMessageDirection   string
	LastMessageStatus      string
	RelationshipUpdatedAt  time.Time
}

type LinkedIMAPMessage struct {
	AccountID             uuid.UUID
	MessageUID            int64
	Subject               string
	FromName              *string
	FromAddress           *string
	SentAt                *time.Time
	ReceivedAt            *time.Time
	RelationshipUpdatedAt time.Time
}

func (r *Repository) ListLinkedWhatsAppConversations(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LinkedWhatsAppConversation, error) {
	const query = `
		SELECT id, phone_number, display_name, last_message_preview, last_message_at, last_message_direction, last_message_status, updated_at
		FROM RAC_whatsapp_conversations
		WHERE organization_id = $1 AND lead_id = $2
		ORDER BY last_message_at DESC NULLS LAST, updated_at DESC`

	rows, err := r.pool.Query(ctx, query, organizationID, leadID)
	if err != nil {
		return nil, fmt.Errorf("list linked whatsapp conversations: %w", err)
	}
	defer rows.Close()

	items := make([]LinkedWhatsAppConversation, 0)
	for rows.Next() {
		var item LinkedWhatsAppConversation
		var lastMessageAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ConversationID,
			&item.PhoneNumber,
			&item.DisplayName,
			&item.LastMessagePreview,
			&lastMessageAt,
			&item.LastMessageDirection,
			&item.LastMessageStatus,
			&item.RelationshipUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan linked whatsapp conversation: %w", err)
		}
		item.LastMessageAt = optionalTime(lastMessageAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked whatsapp conversations: %w", err)
	}
	return items, nil
}

func (r *Repository) ListLinkedIMAPMessages(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LinkedIMAPMessage, error) {
	const query = `
		SELECT l.account_id, l.message_uid, COALESCE(m.subject, ''), m.from_name, m.from_address, m.sent_at, m.received_at, l.updated_at
		FROM RAC_user_imap_message_leads l
		LEFT JOIN RAC_user_imap_messages m
			ON m.account_id = l.account_id AND m.uid = l.message_uid
		WHERE l.organization_id = $1 AND l.lead_id = $2
		ORDER BY COALESCE(m.received_at, l.updated_at) DESC, l.updated_at DESC`

	rows, err := r.pool.Query(ctx, query, organizationID, leadID)
	if err != nil {
		return nil, fmt.Errorf("list linked imap messages: %w", err)
	}
	defer rows.Close()

	items := make([]LinkedIMAPMessage, 0)
	for rows.Next() {
		var item LinkedIMAPMessage
		var fromName pgtype.Text
		var fromAddress pgtype.Text
		var sentAt pgtype.Timestamptz
		var receivedAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.AccountID,
			&item.MessageUID,
			&item.Subject,
			&fromName,
			&fromAddress,
			&sentAt,
			&receivedAt,
			&item.RelationshipUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan linked imap message: %w", err)
		}
		item.FromName = optionalString(fromName)
		item.FromAddress = optionalString(fromAddress)
		item.SentAt = optionalTime(sentAt)
		item.ReceivedAt = optionalTime(receivedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked imap messages: %w", err)
	}
	return items, nil
}