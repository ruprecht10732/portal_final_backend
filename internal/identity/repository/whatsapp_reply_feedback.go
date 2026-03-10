package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	identitydb "portal_final_backend/internal/identity/db"
)

type WhatsAppReplyFeedback struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ConversationID  uuid.UUID
	LeadID          uuid.UUID
	LeadServiceID   uuid.UUID
	AIReply         string
	HumanReply      string
	AppliedToMemory bool
	CreatedAt       time.Time
}

type CreateWhatsAppReplyFeedbackParams struct {
	OrganizationID uuid.UUID
	ConversationID uuid.UUID
	LeadID         uuid.UUID
	AIReply        string
	HumanReply     string
}

func (r *Repository) CreateWhatsAppReplyFeedback(ctx context.Context, params CreateWhatsAppReplyFeedbackParams) (*WhatsAppReplyFeedback, error) {
	if params.OrganizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if params.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation_id is required")
	}
	if params.LeadID == uuid.Nil {
		return nil, fmt.Errorf("lead_id is required")
	}
	params.AIReply = strings.TrimSpace(params.AIReply)
	params.HumanReply = strings.TrimSpace(params.HumanReply)
	if params.AIReply == "" || params.HumanReply == "" || params.AIReply == params.HumanReply {
		return nil, nil
	}

	row, err := r.queries.CreateWhatsAppReplyFeedback(ctx, identitydb.CreateWhatsAppReplyFeedbackParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		ConversationID: toPgUUID(params.ConversationID),
		LeadID:         toPgUUID(params.LeadID),
		AiReply:        params.AIReply,
		HumanReply:     params.HumanReply,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	feedback := whatsAppReplyFeedbackFromDB(row)
	return &feedback, nil
}

func (r *Repository) ListRecentAppliedWhatsAppReplyFeedback(ctx context.Context, organizationID uuid.UUID, referenceLeadID *uuid.UUID, excludeConversationID uuid.UUID, limit int) ([]WhatsAppReplyFeedback, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if referenceLeadID == nil || *referenceLeadID == uuid.Nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if limit > 20 {
		limit = 20
	}

	rows, err := r.queries.ListRecentAppliedWhatsAppReplyFeedback(ctx, identitydb.ListRecentAppliedWhatsAppReplyFeedbackParams{
		OrganizationID: toPgUUID(organizationID),
		LeadID:         toPgUUID(*referenceLeadID),
		ConversationID: toPgUUID(excludeConversationID),
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}

	items := make([]WhatsAppReplyFeedback, 0, len(rows))
	for _, row := range rows {
		items = append(items, whatsAppReplyFeedbackFromDB(row))
	}
	return items, nil
}

func whatsAppReplyFeedbackFromDB(row identitydb.RacWhatsappReplyFeedback) WhatsAppReplyFeedback {
	return WhatsAppReplyFeedback{
		ID:              uuid.UUID(row.ID.Bytes),
		OrganizationID:  uuid.UUID(row.OrganizationID.Bytes),
		ConversationID:  uuid.UUID(row.ConversationID.Bytes),
		LeadID:          uuid.UUID(row.LeadID.Bytes),
		LeadServiceID:   uuid.UUID(row.LeadServiceID.Bytes),
		AIReply:         row.AiReply,
		HumanReply:      row.HumanReply,
		AppliedToMemory: row.AppliedToMemory,
		CreatedAt:       row.CreatedAt.Time,
	}
}
