package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

type SendWhatsAppConversationAttachmentInput struct {
	Filename   string
	Base64Data string
	RemoteURL  string
}

type SendWhatsAppConversationMessageInput struct {
	Type            string
	Body            string
	Caption         string
	ViewOnce        bool
	Compress        bool
	IsForwarded     bool
	PushToTalk      bool
	Attachment      *SendWhatsAppConversationAttachmentInput
	ContactName     string
	ContactPhone    string
	Link            string
	Latitude        string
	Longitude       string
	Question        string
	Options         []string
	MaxAnswer       int
	DurationSeconds *int
}

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

func (s *Service) SendWhatsAppConversationMessage(ctx context.Context, organizationID, conversationID uuid.UUID, input SendWhatsAppConversationMessageInput) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, err := s.repo.GetWhatsAppConversation(ctx, organizationID, conversationID)
	if err != nil {
		if err == repository.ErrNotFound {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.NotFound(conversationNotFoundMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	deviceID, err := s.getRequiredWhatsAppDeviceID(ctx, organizationID)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	outgoing, err := buildOutgoingWhatsAppMessage(input)
	if err != nil {
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, err
	}

	result, err := s.sendWhatsAppConversationMessageByType(ctx, deviceID, conversation.PhoneNumber, outgoing)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Validation(whatsappDeviceNotLinkedMsg)
		}
		return repository.WhatsAppConversation{}, repository.WhatsAppMessage{}, apperr.Internal("WhatsApp-bericht kon niet worden verstuurd")
	}

	return s.persistOutgoingWhatsAppMessage(ctx, organizationID, conversation.LeadID, conversation.PhoneNumber, outgoing.Preview, nilIfEmptyString(result.MessageID), outgoing.Metadata)
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
	_, _, err := s.persistOutgoingWhatsAppMessage(ctx, organizationID, leadID, phoneNumber, body, externalMessageID, nil)
	return err
}

func (s *Service) persistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string, metadata json.RawMessage) (repository.WhatsAppConversation, repository.WhatsAppMessage, error) {
	conversation, message, err := s.repo.RecordSentWhatsAppMessage(ctx, repository.WhatsAppOutgoingMessageParams{
		OrganizationID:    organizationID,
		LeadID:            leadID,
		PhoneNumber:       phoneNumber,
		Body:              body,
		ExternalMessageID: externalMessageID,
		Metadata:          metadata,
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

type outgoingWhatsAppMessage struct {
	Type         string
	Preview      string
	Metadata     json.RawMessage
	Attachment   *whatsapp.MediaAttachment
	RemoteURL    string
	Caption      string
	Body         string
	ViewOnce     bool
	Compress     bool
	IsForwarded  bool
	PushToTalk   bool
	ContactName  string
	ContactPhone string
	Link         string
	Latitude     string
	Longitude    string
	Question     string
	Options      []string
	MaxAnswer    int
	Duration     *int
}

func buildOutgoingWhatsAppMessage(input SendWhatsAppConversationMessageInput) (outgoingWhatsAppMessage, error) {
	messageType := strings.ToLower(strings.TrimSpace(input.Type))
	if messageType == "" {
		messageType = "text"
	}

	message := normalizeOutgoingWhatsAppMessage(input, messageType)

	attachment, remoteURL, err := buildWhatsAppAttachment(input.Attachment)
	if err != nil {
		return outgoingWhatsAppMessage{}, err
	}
	message.Attachment = attachment
	message.RemoteURL = remoteURL

	if err := applyOutgoingWhatsAppMessageTypeRules(&message); err != nil {
		return outgoingWhatsAppMessage{}, err
	}

	metadata, err := marshalOutgoingWhatsAppMetadata(message, input.Attachment)
	if err != nil {
		return outgoingWhatsAppMessage{}, apperr.Internal("WhatsApp-berichtmetadata kon niet worden opgebouwd")
	}
	message.Metadata = metadata
	return message, nil
}

func normalizeOutgoingWhatsAppMessage(input SendWhatsAppConversationMessageInput, messageType string) outgoingWhatsAppMessage {
	return outgoingWhatsAppMessage{
		Type:         messageType,
		Caption:      strings.TrimSpace(input.Caption),
		Body:         strings.TrimSpace(input.Body),
		ViewOnce:     input.ViewOnce,
		Compress:     input.Compress,
		IsForwarded:  input.IsForwarded,
		PushToTalk:   input.PushToTalk,
		ContactName:  strings.TrimSpace(input.ContactName),
		ContactPhone: strings.TrimSpace(input.ContactPhone),
		Link:         strings.TrimSpace(input.Link),
		Latitude:     strings.TrimSpace(input.Latitude),
		Longitude:    strings.TrimSpace(input.Longitude),
		Question:     strings.TrimSpace(input.Question),
		Options:      trimWhatsAppOptions(input.Options),
		MaxAnswer:    input.MaxAnswer,
		Duration:     input.DurationSeconds,
	}
}

func trimWhatsAppOptions(options []string) []string {
	trimmedOptions := make([]string, 0, len(options))
	for _, option := range options {
		trimmed := strings.TrimSpace(option)
		if trimmed != "" {
			trimmedOptions = append(trimmedOptions, trimmed)
		}
	}
	return trimmedOptions
}

func applyOutgoingWhatsAppMessageTypeRules(message *outgoingWhatsAppMessage) error {
	switch message.Type {
	case "text":
		return applyTextMessageRules(message)
	case "image":
		return applyMediaMessageRules(message, "[Afbeelding]")
	case "video":
		return applyMediaMessageRules(message, "[Video]")
	case "audio":
		return applyAudioMessageRules(message)
	case "file":
		return applyMediaMessageRules(message, "[Bestand]")
	case "sticker":
		return applyStickerMessageRules(message)
	case "contact":
		return applyContactMessageRules(message)
	case "link":
		return applyLinkMessageRules(message)
	case "location":
		return applyLocationMessageRules(message)
	case "poll":
		return applyPollMessageRules(message)
	default:
		return apperr.Validation("ongeldig WhatsApp-berichttype")
	}
}

func applyTextMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Body == "" {
		return apperr.Validation("WhatsApp-bericht is leeg")
	}
	message.Preview = message.Body
	return nil
}

func applyMediaMessageRules(message *outgoingWhatsAppMessage, label string) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = previewWithCaption(label, message.Caption)
	return nil
}

func applyAudioMessageRules(message *outgoingWhatsAppMessage) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = "[Audio]"
	if message.PushToTalk {
		message.Preview = "[Spraakbericht]"
	}
	return nil
}

func applyStickerMessageRules(message *outgoingWhatsAppMessage) error {
	if err := requireAttachmentOrRemoteURL(message.Type, message.Attachment, message.RemoteURL); err != nil {
		return err
	}
	message.Preview = "[Sticker]"
	return nil
}

func applyContactMessageRules(message *outgoingWhatsAppMessage) error {
	if message.ContactName == "" || message.ContactPhone == "" {
		return apperr.Validation("contactnaam en contacttelefoon zijn verplicht")
	}
	message.Preview = fmt.Sprintf("[Contact] %s", message.ContactName)
	return nil
}

func applyLinkMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Link == "" {
		return apperr.Validation("link is verplicht")
	}
	message.Preview = message.Link
	if message.Caption != "" {
		message.Preview = previewWithCaption("[Link]", message.Caption)
	}
	return nil
}

func applyLocationMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Latitude == "" || message.Longitude == "" {
		return apperr.Validation("latitude en longitude zijn verplicht")
	}
	message.Preview = "[Locatie]"
	return nil
}

func applyPollMessageRules(message *outgoingWhatsAppMessage) error {
	if message.Question == "" {
		return apperr.Validation("poll-vraag is verplicht")
	}
	if len(message.Options) < 2 {
		return apperr.Validation("een poll heeft minimaal twee opties nodig")
	}
	if message.MaxAnswer < 1 || message.MaxAnswer > len(message.Options) {
		return apperr.Validation("ongeldig maximaal aantal poll-antwoorden")
	}
	message.Preview = fmt.Sprintf("[Poll] %s", message.Question)
	return nil
}

func buildWhatsAppAttachment(input *SendWhatsAppConversationAttachmentInput) (*whatsapp.MediaAttachment, string, error) {
	if input == nil {
		return nil, "", nil
	}

	remoteURL := strings.TrimSpace(input.RemoteURL)
	encoded := strings.TrimSpace(input.Base64Data)
	if encoded == "" {
		return nil, remoteURL, nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, "", apperr.Validation("attachment base64Data is ongeldig")
	}

	return &whatsapp.MediaAttachment{
		Filename: strings.TrimSpace(input.Filename),
		Data:     data,
	}, remoteURL, nil
}

func requireAttachmentOrRemoteURL(messageType string, attachment *whatsapp.MediaAttachment, remoteURL string) error {
	if attachment == nil && strings.TrimSpace(remoteURL) == "" {
		return apperr.Validation(fmt.Sprintf("%s vereist een attachment of remoteUrl", messageType))
	}
	return nil
}

func previewWithCaption(label string, caption string) string {
	if strings.TrimSpace(caption) == "" {
		return label
	}
	return label + " " + strings.TrimSpace(caption)
}

func marshalOutgoingWhatsAppMetadata(message outgoingWhatsAppMessage, attachmentInput *SendWhatsAppConversationAttachmentInput) (json.RawMessage, error) {
	portal := map[string]any{
		"messageType": message.Type,
	}
	addOutgoingTextMetadata(portal, message)
	addOutgoingFlagMetadata(portal, message)
	addOutgoingStructuredMetadata(portal, message)
	addOutgoingAttachmentMetadata(portal, attachmentInput)

	metadata := map[string]any{"portal": portal}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func addOutgoingTextMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.Caption != "" {
		portal["caption"] = message.Caption
	}
	if message.Body != "" && message.Type == "text" {
		portal["text"] = message.Body
	}
}

func addOutgoingFlagMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.ViewOnce {
		portal["viewOnce"] = true
	}
	if message.Compress {
		portal["compress"] = true
	}
	if message.IsForwarded {
		portal["isForwarded"] = true
	}
	if message.PushToTalk {
		portal["pushToTalk"] = true
	}
	if message.Duration != nil {
		portal["durationSeconds"] = *message.Duration
	}
}

func addOutgoingStructuredMetadata(portal map[string]any, message outgoingWhatsAppMessage) {
	if message.ContactName != "" || message.ContactPhone != "" {
		portal["contact"] = map[string]any{
			"name":  message.ContactName,
			"phone": message.ContactPhone,
		}
	}
	if message.Link != "" {
		portal["link"] = message.Link
	}
	if message.Latitude != "" || message.Longitude != "" {
		portal["location"] = map[string]any{
			"latitude":  message.Latitude,
			"longitude": message.Longitude,
		}
	}
	if message.Question != "" {
		portal["poll"] = map[string]any{
			"question":  message.Question,
			"options":   message.Options,
			"maxAnswer": message.MaxAnswer,
		}
	}
}

func addOutgoingAttachmentMetadata(portal map[string]any, attachmentInput *SendWhatsAppConversationAttachmentInput) {
	if attachmentInput == nil {
		return
	}

	attachment := map[string]any{}
	if strings.TrimSpace(attachmentInput.Filename) != "" {
		attachment["filename"] = strings.TrimSpace(attachmentInput.Filename)
	}
	if strings.TrimSpace(attachmentInput.RemoteURL) != "" {
		attachment["remoteUrl"] = strings.TrimSpace(attachmentInput.RemoteURL)
	}
	if strings.TrimSpace(attachmentInput.Base64Data) != "" {
		attachment["hasInlineData"] = true
	}
	if len(attachment) > 0 {
		portal["attachment"] = attachment
	}
}

func (s *Service) sendWhatsAppConversationMessageByType(ctx context.Context, deviceID string, phoneNumber string, message outgoingWhatsAppMessage) (whatsapp.SendResult, error) {
	switch message.Type {
	case "text":
		return s.whatsapp.SendMessage(ctx, deviceID, phoneNumber, message.Body)
	case "image":
		return s.whatsapp.SendImage(ctx, deviceID, whatsapp.SendImageInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			ViewOnce:        message.ViewOnce,
			Compress:        message.Compress,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "video":
		return s.whatsapp.SendVideo(ctx, deviceID, whatsapp.SendVideoInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			ViewOnce:        message.ViewOnce,
			Compress:        message.Compress,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "audio":
		return s.whatsapp.SendAudio(ctx, deviceID, whatsapp.SendAudioInput{
			PhoneNumber:     phoneNumber,
			IsForwarded:     message.IsForwarded,
			PTT:             message.PushToTalk,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "file":
		return s.whatsapp.SendFile(ctx, deviceID, whatsapp.SendFileInput{
			PhoneNumber:     phoneNumber,
			Caption:         message.Caption,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "sticker":
		return s.whatsapp.SendSticker(ctx, deviceID, whatsapp.SendStickerInput{
			PhoneNumber:     phoneNumber,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
			Attachment:      message.Attachment,
			RemoteURL:       message.RemoteURL,
		})
	case "contact":
		return s.whatsapp.SendContact(ctx, deviceID, whatsapp.SendContactInput{
			PhoneNumber:     phoneNumber,
			ContactName:     message.ContactName,
			ContactPhone:    message.ContactPhone,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "link":
		return s.whatsapp.SendLink(ctx, deviceID, whatsapp.SendLinkInput{
			PhoneNumber:     phoneNumber,
			Link:            message.Link,
			Caption:         message.Caption,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "location":
		return s.whatsapp.SendLocation(ctx, deviceID, whatsapp.SendLocationInput{
			PhoneNumber:     phoneNumber,
			Latitude:        message.Latitude,
			Longitude:       message.Longitude,
			IsForwarded:     message.IsForwarded,
			DurationSeconds: message.Duration,
		})
	case "poll":
		return s.whatsapp.SendPoll(ctx, deviceID, whatsapp.SendPollInput{
			PhoneNumber:     phoneNumber,
			Question:        message.Question,
			Options:         message.Options,
			MaxAnswer:       message.MaxAnswer,
			DurationSeconds: message.Duration,
		})
	default:
		return whatsapp.SendResult{}, apperr.Validation("ongeldig WhatsApp-berichttype")
	}
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
