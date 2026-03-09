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
	ApplyWhatsAppMessageReceipt(ctx context.Context, organizationID uuid.UUID, externalMessageIDs []string, receiptType string, receiptAt *time.Time) (bool, error)
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
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "outbound echo"})
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
