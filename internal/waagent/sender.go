package waagent

import (
	"context"
	"log"
	"strings"

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
	cfg, err := s.getAgentConfig(ctx)
	if err != nil {
		log.Printf("waagent: no agent device configured: %v", err)
		return err
	}

	result, err := s.client.SendMessage(ctx, cfg.DeviceID, phone, text)
	if err != nil {
		return err
	}

	// Write to inbox for operator visibility
	if s.inboxWriter != nil && orgID != uuid.Nil {
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

func (s *Sender) SendChatPresence(ctx context.Context, phone string, action whatsapp.ChatPresenceAction) error {
	if s.client == nil {
		return nil
	}
	if strings.TrimSpace(phone) == "" {
		return nil
	}
	cfg, err := s.getAgentConfig(ctx)
	if err != nil {
		log.Printf("waagent: no agent device configured for chat presence: %v", err)
		return err
	}
	return s.client.SendChatPresence(ctx, cfg.DeviceID, phone, string(action))
}

func (s *Sender) SendFileReply(ctx context.Context, orgID uuid.UUID, phone, caption, fileName string, data []byte) error {
	if s.client == nil {
		return nil
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "bestand.pdf"
	}
	cfg, err := s.getAgentConfig(ctx)
	if err != nil {
		log.Printf("waagent: no agent device configured for file send: %v", err)
		return err
	}
	result, err := s.client.SendFile(ctx, cfg.DeviceID, whatsapp.SendFileInput{
		PhoneNumber: phone,
		Caption:     strings.TrimSpace(caption),
		Attachment: &whatsapp.MediaAttachment{
			Filename: fileName,
			Data:     data,
		},
	})
	if err != nil {
		return err
	}
	if s.inboxWriter != nil && orgID != uuid.Nil {
		var msgID *string
		if result.MessageID != "" {
			msgID = &result.MessageID
		}
		body := strings.TrimSpace(caption)
		if body == "" {
			body = fileName
		}
		if persistErr := s.inboxWriter.PersistOutgoingWhatsAppMessage(ctx, orgID, nil, phone, body, msgID); persistErr != nil {
			log.Printf("waagent: inbox persist error for file phone=%s org=%s: %v", phone, orgID, persistErr)
		}
	}
	return nil
}

func (s *Sender) getAgentConfig(ctx context.Context) (waagentdb.RacWhatsappAgentConfig, error) {
	return s.queries.GetAgentConfig(ctx)
}
