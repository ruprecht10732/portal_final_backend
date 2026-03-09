package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type WhatsAppInboxIngester interface {
	ReceiveIncomingWhatsAppMessage(ctx context.Context, message IncomingWhatsAppMessage) (bool, error)
	SyncOutgoingWhatsAppMessage(ctx context.Context, message OutgoingWhatsAppMessage) (bool, error)
	ApplyWhatsAppMessageReceipt(ctx context.Context, organizationID uuid.UUID, externalMessageIDs []string, receiptType string, receiptAt *time.Time) (bool, error)
	ApplyWhatsAppMessageMutation(ctx context.Context, message WhatsAppMessageMutation) (bool, error)
}

type IncomingWhatsAppMessage struct {
	OrganizationID    uuid.UUID
	PhoneNumber       string
	DisplayName       string
	ExternalMessageID *string
	Body              string
	Metadata          json.RawMessage
	ReceivedAt        *time.Time
}

type OutgoingWhatsAppMessage struct {
	OrganizationID    uuid.UUID
	PhoneNumber       string
	DisplayName       string
	ExternalMessageID *string
	Body              string
	Metadata          json.RawMessage
	SentAt            *time.Time
}

type WhatsAppMessageMutation struct {
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

type WhatsAppWebhookEnvelope struct {
	Event     string          `json:"event"`
	DeviceID  string          `json:"device_id"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type whatsAppWebhookPayload struct {
	ID        string `json:"id"`
	ChatID    string `json:"chat_id"`
	From      string `json:"from"`
	FromName  string `json:"from_name"`
	Timestamp string `json:"timestamp"`
	IsFromMe  bool   `json:"is_from_me"`
	Body      string `json:"body"`
}

type whatsAppAckPayload struct {
	IDs         []string `json:"ids"`
	ChatID      string   `json:"chat_id"`
	From        string   `json:"from"`
	ReceiptType string   `json:"receipt_type"`
	Timestamp   string   `json:"timestamp"`
}

type whatsAppReactionPayload struct {
	ID               string `json:"id"`
	ChatID           string `json:"chat_id"`
	From             string `json:"from"`
	FromName         string `json:"from_name"`
	Timestamp        string `json:"timestamp"`
	IsFromMe         bool   `json:"is_from_me"`
	Reaction         string `json:"reaction"`
	ReactedMessageID string `json:"reacted_message_id"`
}

type whatsAppEditedPayload struct {
	ID                string `json:"id"`
	ChatID            string `json:"chat_id"`
	From              string `json:"from"`
	FromName          string `json:"from_name"`
	Timestamp         string `json:"timestamp"`
	IsFromMe          bool   `json:"is_from_me"`
	OriginalMessageID string `json:"original_message_id"`
	Body              string `json:"body"`
}

type whatsAppRevokedPayload struct {
	ID               string `json:"id"`
	ChatID           string `json:"chat_id"`
	From             string `json:"from"`
	FromName         string `json:"from_name"`
	Timestamp        string `json:"timestamp"`
	IsFromMe         bool   `json:"is_from_me"`
	RevokedMessageID string `json:"revoked_message_id"`
	RevokedFromMe    bool   `json:"revoked_from_me"`
	RevokedChat      string `json:"revoked_chat"`
}

type whatsAppDeletedPayload struct {
	DeletedMessageID  string `json:"deleted_message_id"`
	Timestamp         string `json:"timestamp"`
	From              string `json:"from"`
	ChatID            string `json:"chat_id"`
	OriginalContent   string `json:"original_content"`
	OriginalSender    string `json:"original_sender"`
	OriginalTimestamp string `json:"original_timestamp"`
	WasFromMe         bool   `json:"was_from_me"`
}

const (
	whatsAppDeletedPlaceholder = "[Bericht verwijderd]"
	whatsAppRevokedPlaceholder = "[Bericht verwijderd voor iedereen]"
)

type WhatsAppWebhookResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) HandleWhatsAppWebhook(c *gin.Context) {
	orgID, ok := h.getWebhookOrgID(c)
	if !ok {
		return
	}
	if h.whatsappInbox == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "whatsapp inbox ingestion is not configured", nil)
		return
	}

	var request WhatsAppWebhookEnvelope
	if err := c.ShouldBindJSON(&request); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid payload", err.Error())
		return
	}

	switch request.Event {
	case "message":
		h.handleIncomingWhatsAppMessage(c, orgID, request)
	case "message.ack":
		h.handleWhatsAppReceipt(c, orgID, request)
	case "message.reaction", "message.edited", "message.revoked", "message.deleted":
		h.handleWhatsAppMessageMutation(c, orgID, request)
	default:
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "unsupported event"})
	}
}

func (h *Handler) handleIncomingWhatsAppMessage(c *gin.Context, orgID uuid.UUID, request WhatsAppWebhookEnvelope) {
	var payload whatsAppWebhookPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid message payload", err.Error())
		return
	}
	if payload.IsFromMe {
		h.handleOutgoingWhatsAppMessage(c, orgID, request, payload)
		return
	}

	messageAddress := strings.TrimSpace(payload.From)
	if messageAddress == "" {
		messageAddress = strings.TrimSpace(payload.ChatID)
	}
	if isNonDirectWhatsAppAddress(messageAddress) || isNonDirectWhatsAppAddress(payload.ChatID) {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "group or non-direct chat"})
		return
	}

	body := extractWhatsAppMessageBody(request.Payload, payload.Body)
	if body == "" {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "empty message body"})
		return
	}

	metadata, err := json.Marshal(request)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to encode metadata", nil)
		return
	}

	created, err := h.whatsappInbox.ReceiveIncomingWhatsAppMessage(c.Request.Context(), IncomingWhatsAppMessage{
		OrganizationID:    orgID,
		PhoneNumber:       messageAddress,
		DisplayName:       payload.FromName,
		ExternalMessageID: optionalTrimmedString(payload.ID),
		Body:              body,
		Metadata:          metadata,
		ReceivedAt:        parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp),
	})
	if httpkit.HandleError(c, err) {
		return
	}
	if !created {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "duplicate"})
		return
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func (h *Handler) handleOutgoingWhatsAppMessage(c *gin.Context, orgID uuid.UUID, request WhatsAppWebhookEnvelope, payload whatsAppWebhookPayload) {
	messageAddress := strings.TrimSpace(payload.ChatID)
	if messageAddress == "" {
		messageAddress = strings.TrimSpace(payload.From)
	}
	if isNonDirectWhatsAppAddress(messageAddress) || isNonDirectWhatsAppAddress(payload.ChatID) {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "group or non-direct chat"})
		return
	}

	body := extractWhatsAppMessageBody(request.Payload, payload.Body)
	if body == "" {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "empty message body"})
		return
	}

	metadata, err := json.Marshal(request)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to encode metadata", nil)
		return
	}

	created, err := h.whatsappInbox.SyncOutgoingWhatsAppMessage(c.Request.Context(), OutgoingWhatsAppMessage{
		OrganizationID:    orgID,
		PhoneNumber:       messageAddress,
		DisplayName:       payload.FromName,
		ExternalMessageID: optionalTrimmedString(payload.ID),
		Body:              body,
		Metadata:          metadata,
		SentAt:            parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp),
	})
	if httpkit.HandleError(c, err) {
		return
	}
	if !created {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "duplicate"})
		return
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func (h *Handler) handleWhatsAppReceipt(c *gin.Context, orgID uuid.UUID, request WhatsAppWebhookEnvelope) {
	var payload whatsAppAckPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid receipt payload", err.Error())
		return
	}
	if strings.TrimSpace(payload.ReceiptType) == "" || len(payload.IDs) == 0 {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "empty receipt payload"})
		return
	}

	applied, err := h.whatsappInbox.ApplyWhatsAppMessageReceipt(c.Request.Context(), orgID, payload.IDs, payload.ReceiptType, parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp))
	if httpkit.HandleError(c, err) {
		return
	}
	if !applied {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "unsupported or unmatched receipt"})
		return
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func (h *Handler) handleWhatsAppMessageMutation(c *gin.Context, orgID uuid.UUID, request WhatsAppWebhookEnvelope) {
	mutation, ok, err := buildWhatsAppMessageMutation(orgID, request)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid mutation payload", err.Error())
		return
	}
	if !ok {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "unsupported or non-direct mutation"})
		return
	}

	applied, err := h.whatsappInbox.ApplyWhatsAppMessageMutation(c.Request.Context(), mutation)
	if httpkit.HandleError(c, err) {
		return
	}
	if !applied {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "unmatched message"})
		return
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func buildWhatsAppMessageMutation(orgID uuid.UUID, request WhatsAppWebhookEnvelope) (WhatsAppMessageMutation, bool, error) {
	metadata, err := json.Marshal(request)
	if err != nil {
		return WhatsAppMessageMutation{}, false, err
	}

	switch request.Event {
	case "message.reaction":
		return buildReactionMutation(orgID, request, metadata)
	case "message.edited":
		return buildEditedMutation(orgID, request, metadata)
	case "message.revoked":
		return buildRevokedMutation(orgID, request, metadata)
	case "message.deleted":
		return buildDeletedMutation(orgID, request, metadata)
	default:
		return WhatsAppMessageMutation{}, false, nil
	}
}

func buildReactionMutation(orgID uuid.UUID, request WhatsAppWebhookEnvelope, metadata json.RawMessage) (WhatsAppMessageMutation, bool, error) {
	var payload whatsAppReactionPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return WhatsAppMessageMutation{}, false, err
	}

	phoneNumber := resolveDirectWhatsAppAddress(payload.ChatID, payload.From)
	if phoneNumber == "" || strings.TrimSpace(payload.ReactedMessageID) == "" {
		return WhatsAppMessageMutation{}, false, nil
	}

	return WhatsAppMessageMutation{
		OrganizationID:          orgID,
		EventType:               request.Event,
		TargetExternalMessageID: strings.TrimSpace(payload.ReactedMessageID),
		PhoneNumber:             phoneNumber,
		ActorJID:                strings.TrimSpace(payload.From),
		ActorName:               strings.TrimSpace(payload.FromName),
		EventMessageID:          optionalTrimmedString(payload.ID),
		Reaction:                optionalTrimmedString(payload.Reaction),
		Metadata:                metadata,
		OccurredAt:              parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp),
		IsFromMe:                boolPointer(payload.IsFromMe),
	}, true, nil
}

func buildEditedMutation(orgID uuid.UUID, request WhatsAppWebhookEnvelope, metadata json.RawMessage) (WhatsAppMessageMutation, bool, error) {
	var payload whatsAppEditedPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return WhatsAppMessageMutation{}, false, err
	}

	phoneNumber := resolveDirectWhatsAppAddress(payload.ChatID, payload.From)
	body := strings.TrimSpace(payload.Body)
	if phoneNumber == "" || strings.TrimSpace(payload.OriginalMessageID) == "" || body == "" {
		return WhatsAppMessageMutation{}, false, nil
	}

	return WhatsAppMessageMutation{
		OrganizationID:          orgID,
		EventType:               request.Event,
		TargetExternalMessageID: strings.TrimSpace(payload.OriginalMessageID),
		PhoneNumber:             phoneNumber,
		ActorJID:                strings.TrimSpace(payload.From),
		ActorName:               strings.TrimSpace(payload.FromName),
		EventMessageID:          optionalTrimmedString(payload.ID),
		Body:                    &body,
		Metadata:                metadata,
		OccurredAt:              parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp),
		IsFromMe:                boolPointer(payload.IsFromMe),
	}, true, nil
}

func buildRevokedMutation(orgID uuid.UUID, request WhatsAppWebhookEnvelope, metadata json.RawMessage) (WhatsAppMessageMutation, bool, error) {
	var payload whatsAppRevokedPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return WhatsAppMessageMutation{}, false, err
	}

	phoneNumber := resolveDirectWhatsAppAddress(payload.ChatID, payload.From, payload.RevokedChat)
	if phoneNumber == "" || strings.TrimSpace(payload.RevokedMessageID) == "" {
		return WhatsAppMessageMutation{}, false, nil
	}
	body := whatsAppRevokedPlaceholder

	return WhatsAppMessageMutation{
		OrganizationID:          orgID,
		EventType:               request.Event,
		TargetExternalMessageID: strings.TrimSpace(payload.RevokedMessageID),
		PhoneNumber:             phoneNumber,
		ActorJID:                strings.TrimSpace(payload.From),
		ActorName:               strings.TrimSpace(payload.FromName),
		EventMessageID:          optionalTrimmedString(payload.ID),
		Body:                    &body,
		Metadata:                metadata,
		OccurredAt:              parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp),
		IsFromMe:                boolPointer(payload.IsFromMe),
	}, true, nil
}

func buildDeletedMutation(orgID uuid.UUID, request WhatsAppWebhookEnvelope, metadata json.RawMessage) (WhatsAppMessageMutation, bool, error) {
	var payload whatsAppDeletedPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return WhatsAppMessageMutation{}, false, err
	}

	phoneNumber := resolveDirectWhatsAppAddress(payload.ChatID, payload.From, payload.OriginalSender)
	if phoneNumber == "" || strings.TrimSpace(payload.DeletedMessageID) == "" {
		return WhatsAppMessageMutation{}, false, nil
	}
	body := whatsAppDeletedPlaceholder

	return WhatsAppMessageMutation{
		OrganizationID:          orgID,
		EventType:               request.Event,
		TargetExternalMessageID: strings.TrimSpace(payload.DeletedMessageID),
		PhoneNumber:             phoneNumber,
		ActorJID:                strings.TrimSpace(payload.From),
		EventMessageID:          optionalTrimmedString(payload.DeletedMessageID),
		Body:                    &body,
		Metadata:                metadata,
		OccurredAt:              parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp, payload.OriginalTimestamp),
		IsFromMe:                boolPointer(payload.WasFromMe),
	}, true, nil
}

func resolveDirectWhatsAppAddress(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || isNonDirectWhatsAppAddress(trimmed) {
			continue
		}
		return trimmed
	}
	return ""
}

func boolPointer(value bool) *bool {
	return &value
}

func parseWhatsAppWebhookTimestamp(values ...string) *time.Time {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func isNonDirectWhatsAppAddress(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "@g.us") || strings.Contains(trimmed, "@newsletter") || strings.Contains(trimmed, "status@broadcast")
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func extractWhatsAppMessageBody(rawPayload json.RawMessage, fallbackBody string) string {
	if trimmed := strings.TrimSpace(fallbackBody); trimmed != "" {
		return trimmed
	}

	var payload map[string]any
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return ""
	}

	structuredFields := []struct {
		key   string
		label string
	}{
		{key: "image", label: "[Image]"},
		{key: "video", label: "[Video]"},
		{key: "audio", label: "[Audio]"},
		{key: "document", label: "[Document]"},
		{key: "sticker", label: "[Sticker]"},
		{key: "video_note", label: "[Video note]"},
		{key: "location", label: "[Location]"},
		{key: "contact", label: "[Contact]"},
	}

	for _, field := range structuredFields {
		if body := messageBodyFromStructuredField(payload, field.key, field.label); body != "" {
			return body
		}
	}

	return ""
}

func messageBodyFromStructuredField(payload map[string]any, key string, label string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return ""
		}
		return label
	case map[string]any:
		return messageBodyFromStructuredObject(typed, label)
	default:
		return label
	}
}

func messageBodyFromStructuredObject(value map[string]any, label string) string {
	if caption, ok := value["caption"].(string); ok && strings.TrimSpace(caption) != "" {
		return strings.TrimSpace(caption)
	}
	if filename, ok := value["filename"].(string); ok && strings.TrimSpace(filename) != "" {
		return fmt.Sprintf("%s %s", label, strings.TrimSpace(filename))
	}
	return label
}
