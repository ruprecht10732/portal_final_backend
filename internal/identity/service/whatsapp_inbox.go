package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/notification/sse"
	webhookinbox "portal_final_backend/internal/webhook"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const conversationNotFoundMsg = "conversation not found"
const whatsappDeviceNotLinkedMsg = "er is geen verbonden WhatsApp-apparaat voor deze organisatie"

func (s *Service) SetSSE(sseService *sse.Service) {
	s.sse = sseService
}

func (s *Service) ListWhatsAppConversations(ctx context.Context, organizationID uuid.UUID, limit, offset int) ([]repository.WhatsAppConversation, error) {
	return s.repo.ListWhatsAppConversations(ctx, organizationID, limit, offset)
}

func (s *Service) GetWhatsAppConversationMessages(ctx context.Context, organizationID, conversationID uuid.UUID, limit int) (repository.WhatsAppConversation, []repository.WhatsAppMessage, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, nil, apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, nil, err
	}

	messages, err := s.repo.ListWhatsAppMessages(ctx, organizationID, conversationID, limit)
	if err != nil {
		return repository.WhatsAppConversation{}, nil, err
	}

	return conversation, messages, nil
}

func (s *Service) MarkWhatsAppConversationRead(ctx context.Context, organizationID, conversationID uuid.UUID) (bool, error) {
	providerSynced := false
	readTarget, err := s.repo.GetLatestUnreadWhatsAppReadSyncTarget(ctx, organizationID, conversationID)
	if err != nil {
		return false, err
	}
	if readTarget != nil {
		deviceID, deviceErr := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
		if deviceErr == nil {
			if providerErr := s.whatsapp.MarkMessageRead(ctx, deviceID, readTarget.PhoneNumber, readTarget.ExternalMessageID); providerErr == nil {
				providerSynced = true
			}
		}
	}

	err = s.repo.MarkWhatsAppConversationRead(ctx, organizationID, conversationID)
	if err == repository.ErrNotFound {
		return false, apperr.NotFound(conversationNotFoundMsg)
	}
	if err != nil {
		return false, err
	}

	conversation, convErr := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if convErr == nil {
		s.publishWhatsAppConversationUpdated(organizationID, conversation)
	}

	return providerSynced, nil
}

func (s *Service) SendWhatsAppConversationMessage(ctx context.Context, organizationID, conversationID uuid.UUID, body string) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	messageBody := strings.TrimSpace(body)
	if messageBody == "" {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation("WhatsApp-bericht is leeg")
	}
	if s.whatsapp == nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Internal(whatsappNotConfiguredMsg)
	}

	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}
	if settings.WhatsAppDeviceID == nil || strings.TrimSpace(*settings.WhatsAppDeviceID) == "" {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation(whatsappDeviceNotLinkedMsg)
	}

	result, err := s.whatsapp.SendMessage(ctx, strings.TrimSpace(*settings.WhatsAppDeviceID), conversation.PhoneNumber, messageBody)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation(whatsappDeviceNotLinkedMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Internal("WhatsApp-bericht kon niet worden verstuurd")
	}

	return s.persistOutgoingWhatsAppMessage(ctx, organizationID, conversation.LeadID, conversation.PhoneNumber, messageBody, nilIfEmptyString(result.MessageID))
}

func (s *Service) ReceiveIncomingWhatsAppMessage(ctx context.Context, input webhookinbox.IncomingWhatsAppMessage) (bool, error) {
	conversation, message, created, err := s.repo.RecordIncomingWhatsAppMessage(ctx, repository.WhatsAppIncomingMessageParams{
		OrganizationID:    input.OrganizationID,
		PhoneNumber:       input.PhoneNumber,
		DisplayName:       input.DisplayName,
		ExternalMessageID: input.ExternalMessageID,
		Body:              input.Body,
		Metadata:          input.Metadata,
		ReceivedAt:        input.ReceivedAt,
	})
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	s.publishWhatsAppMessageReceived(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) SyncOutgoingWhatsAppMessage(ctx context.Context, input webhookinbox.OutgoingWhatsAppMessage) (bool, error) {
	conversation, message, created, err := s.repo.SyncSentWhatsAppMessage(ctx, repository.WhatsAppOutgoingMessageParams{
		OrganizationID:    input.OrganizationID,
		PhoneNumber:       input.PhoneNumber,
		Body:              input.Body,
		ExternalMessageID: input.ExternalMessageID,
		Metadata:          input.Metadata,
		SentAt:            input.SentAt,
	})
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}

	s.publishWhatsAppMessageSent(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) CountUnreadWhatsAppConversations(ctx context.Context, organizationID uuid.UUID) (int, error) {
	return s.repo.CountUnreadWhatsAppConversations(ctx, organizationID)
}

func (s *Service) SendWhatsAppPresence(ctx context.Context, organizationID uuid.UUID, presenceType string) error {
	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return err
	}

	trimmedType := strings.ToLower(strings.TrimSpace(presenceType))
	if trimmedType != "available" && trimmedType != "unavailable" {
		return apperr.Validation("ongeldig presence-type")
	}

	if err := s.whatsapp.SendPresence(ctx, deviceID, trimmedType); err != nil {
		return apperr.Internal("WhatsApp presence kon niet worden verstuurd")
	}
	if _, err := s.repo.UpsertOrganizationSettings(ctx, organizationID, repository.OrganizationSettingsUpdate{
		WhatsAppPresence: &trimmedType,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) SendWhatsAppChatPresence(ctx context.Context, organizationID, conversationID uuid.UUID, action string) error {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return apperr.NotFound(conversationNotFoundMsg)
		}
		return err
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return err
	}

	trimmedAction := strings.ToLower(strings.TrimSpace(action))
	if trimmedAction != "start" && trimmedAction != "stop" {
		return apperr.Validation("ongeldige chat presence-actie")
	}

	if err := s.whatsapp.SendChatPresence(ctx, deviceID, conversation.PhoneNumber, trimmedAction); err != nil {
		return apperr.Internal("WhatsApp typing-indicator kon niet worden verstuurd")
	}
	return nil
}

func (s *Service) PersistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error {
	_, _, err := s.persistOutgoingWhatsAppMessage(ctx, organizationID, leadID, phoneNumber, body, externalMessageID)
	return err
}

func (s *Service) persistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, message, err := s.repo.RecordSentWhatsAppMessage(ctx, repository.WhatsAppOutgoingMessageParams{
		OrganizationID:    organizationID,
		LeadID:            leadID,
		PhoneNumber:       phoneNumber,
		Body:              body,
		ExternalMessageID: externalMessageID,
	})
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	s.publishWhatsAppMessageSent(organizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(organizationID, conversation)

	return conversation, message, nil
}

func (s *Service) ApplyWhatsAppMessageReceipt(ctx context.Context, organizationID uuid.UUID, externalMessageIDs []string, receiptType string, receiptAt *time.Time) (bool, error) {
	conversations, messages, err := s.repo.ApplyWhatsAppMessageReceipt(ctx, organizationID, externalMessageIDs, receiptType, receiptAt)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}

	conversationsByID := make(map[uuid.UUID]repository.WhatsAppConversation, len(conversations))
	for _, conversation := range conversations {
		conversationsByID[conversation.ID] = conversation
	}
	for _, message := range messages {
		conversation, ok := conversationsByID[message.ConversationID]
		if !ok {
			conversation, err = s.repo.GetWhatsAppConversation(ctx, organizationID, message.ConversationID)
			if err != nil {
				continue
			}
		}
		s.publishWhatsAppMessageSent(organizationID, conversation, message)
	}
	for _, conversation := range conversations {
		s.publishWhatsAppConversationUpdated(organizationID, conversation)
	}

	return true, nil
}

func (s *Service) ApplyWhatsAppMessageMutation(ctx context.Context, input webhookinbox.WhatsAppMessageMutation) (bool, error) {
	conversation, message, applied, err := s.repo.ApplyWhatsAppMessageMutation(ctx, repository.WhatsAppMessageMutationParams{
		OrganizationID:          input.OrganizationID,
		EventType:               input.EventType,
		TargetExternalMessageID: input.TargetExternalMessageID,
		PhoneNumber:             input.PhoneNumber,
		ActorJID:                input.ActorJID,
		ActorName:               input.ActorName,
		EventMessageID:          input.EventMessageID,
		Body:                    input.Body,
		Reaction:                input.Reaction,
		Metadata:                input.Metadata,
		OccurredAt:              input.OccurredAt,
		IsFromMe:                input.IsFromMe,
	})
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}

	s.publishWhatsAppMessageUpdated(input.OrganizationID, conversation, message)
	s.publishWhatsAppConversationUpdated(input.OrganizationID, conversation)
	return true, nil
}

func (s *Service) publishWhatsAppMessageSent(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageSent,
		Message: "WhatsApp message sent",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
			},
			"message": map[string]any{
				"id":             message.ID.String(),
				"conversationId": message.ConversationID.String(),
				"leadId":         optionalUUIDString(message.LeadID),
				"direction":      message.Direction,
				"status":         message.Status,
				"phoneNumber":    message.PhoneNumber,
				"body":           message.Body,
				"createdAt":      message.CreatedAt,
				"sentAt":         message.SentAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppMessageReceived(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageReceived,
		Message: "WhatsApp message received",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
			},
			"message": map[string]any{
				"id":                message.ID.String(),
				"conversationId":    message.ConversationID.String(),
				"leadId":            optionalUUIDString(message.LeadID),
				"externalMessageId": message.ExternalMessageID,
				"direction":         message.Direction,
				"status":            message.Status,
				"phoneNumber":       message.PhoneNumber,
				"body":              message.Body,
				"createdAt":         message.CreatedAt,
				"sentAt":            message.SentAt,
				"readAt":            message.ReadAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppConversationUpdated(organizationID uuid.UUID, conversation repository.WhatsAppConversation) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppConversationUpdated,
		Message: "WhatsApp conversation updated",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func (s *Service) publishWhatsAppMessageUpdated(organizationID uuid.UUID, conversation repository.WhatsAppConversation, message repository.WhatsAppMessage) {
	if s.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventWhatsAppMessageUpdated,
		Message: "WhatsApp message updated",
		Data: map[string]any{
			"conversation": map[string]any{
				"id":                   conversation.ID.String(),
				"leadId":               optionalUUIDString(conversation.LeadID),
				"phoneNumber":          conversation.PhoneNumber,
				"displayName":          conversation.DisplayName,
				"lastMessagePreview":   conversation.LastMessagePreview,
				"lastMessageAt":        conversation.LastMessageAt,
				"lastMessageDirection": conversation.LastMessageDirection,
				"lastMessageStatus":    conversation.LastMessageStatus,
				"unreadCount":          conversation.UnreadCount,
			},
			"message": map[string]any{
				"id":                message.ID.String(),
				"conversationId":    message.ConversationID.String(),
				"leadId":            optionalUUIDString(message.LeadID),
				"externalMessageId": message.ExternalMessageID,
				"direction":         message.Direction,
				"status":            message.Status,
				"phoneNumber":       message.PhoneNumber,
				"body":              message.Body,
				"metadata":          message.Metadata,
				"createdAt":         message.CreatedAt,
				"sentAt":            message.SentAt,
				"readAt":            message.ReadAt,
				"failedAt":          message.FailedAt,
			},
		},
	}
	if conversation.LeadID != nil {
		event.LeadID = *conversation.LeadID
	}

	s.sse.PublishToOrganization(organizationID, event)
}

func optionalUUIDString(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nilIfEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *Service) getRequiredWhatsAppDeviceID(ctx context.Context, organizationID uuid.UUID) (string, error) {
	if s.whatsapp == nil {
		return "", apperr.Internal(whatsappNotConfiguredMsg)
	}

	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return "", err
	}
	if settings.WhatsAppDeviceID == nil || strings.TrimSpace(*settings.WhatsAppDeviceID) == "" {
		return "", apperr.Validation(whatsappDeviceNotLinkedMsg)
	}

	return strings.TrimSpace(*settings.WhatsAppDeviceID), nil
}
