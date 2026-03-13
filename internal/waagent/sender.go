package waagent

import (
	"context"
	"log"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

// Sender sends WhatsApp replies via the global agent device and persists them to the operator inbox.
type Sender struct {
	client      *whatsapp.Client
	queries     waagentdb.Querier
	inboxWriter InboxWriter
	log         *logger.Logger
}

// SendReply sends a text message via the global agent device and writes it to the inbox for operator visibility.
func (s *Sender) SendReply(ctx context.Context, orgID uuid.UUID, phone, text string) error {
	if s.client == nil {
		return nil
	}

	// Resolve device ID from global agent config
	cfg, err := s.queries.GetAgentConfig(ctx)
	if err != nil {
		log.Printf("waagent: no agent device configured: %v", err)
		return err
	}

	result, err := s.client.SendMessage(ctx, cfg.DeviceID, phone, text)
	if err != nil {
		return err
	}

	// Write to inbox for operator visibility
	if s.inboxWriter != nil {
		var msgID *string
		if result.MessageID != "" {
			msgID = &result.MessageID
		}

		if persistErr := s.inboxWriter.PersistOutgoingWhatsAppMessage(ctx, orgID, nil, phone, text, msgID); persistErr != nil {
			log.Printf("waagent: inbox persist error phone=%s org=%s: %v", phone, orgID, persistErr)
		}
	}

	return nil
}
