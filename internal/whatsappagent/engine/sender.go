package engine

import (
	"context"
	"strings"

	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

// Sender sends WhatsApp replies via the global agent device and persists them to the operator inbox.
type Sender struct {
	client      WhatsAppTransport
	queries     AgentConfigReader
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
		s.logWarn(ctx, "whatsappagent: no agent device configured", "error", err)
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
			s.logWarn(ctx, "whatsappagent: inbox persist error", "phone", phone, "organization_id", orgID.String(), "error", persistErr)
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
		s.logWarn(ctx, "whatsappagent: no agent device configured for chat presence", "phone", phone, "error", err)
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
		s.logWarn(ctx, "whatsappagent: no agent device configured for file send", "phone", phone, "error", err)
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
			s.logWarn(ctx, "whatsappagent: inbox persist error for file", "phone", phone, "organization_id", orgID.String(), "error", persistErr)
		}
	}
	return nil
}

func (s *Sender) getAgentConfig(ctx context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error) {
	return s.queries.GetAgentConfig(ctx)
}

func (s *Sender) loggerWithContext(ctx context.Context) *logger.Logger {
	if s == nil || s.log == nil {
		return nil
	}
	return s.log.WithContext(ctx)
}

func (s *Sender) logWarn(ctx context.Context, message string, args ...any) {
	if lg := s.loggerWithContext(ctx); lg != nil {
		lg.Warn(message, args...)
	}
}
