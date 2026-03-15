package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/waagent"
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
	ID          string   `json:"id"`
	MessageID   string   `json:"message_id"`
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
	whatsAppInvalidMessagePayload = "invalid message payload"
	whatsAppDeletedPlaceholder    = "[Bericht verwijderd]"
	whatsAppPollPrefix            = "[Poll] "
	whatsAppRevokedPlaceholder    = "[Bericht verwijderd voor iedereen]"
	whatsAppIgnoredNonDirectChat  = "group or non-direct chat"
	whatsAppIgnoredEmptyBody      = "empty message body"
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

	var request WhatsAppWebhookEnvelope
	if err := c.ShouldBindJSON(&request); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid payload", err.Error())
		return
	}

	// Global agent device — only handle messages, route directly to agent.
	if c.GetBool("isAgentDevice") {
		if request.Event == "message" {
			h.handleAgentDeviceMessage(c, request)
		} else {
			httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "agent device only handles messages"})
		}
		return
	}

	if h.whatsappInbox == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "whatsapp inbox ingestion is not configured", nil)
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
		httpkit.Error(c, http.StatusBadRequest, whatsAppInvalidMessagePayload, err.Error())
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
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredNonDirectChat})
		return
	}

	body, metadata, err := buildWhatsAppWebhookMessageData(request, payload.Body)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, whatsAppInvalidMessagePayload, err.Error())
		return
	}
	if body == "" {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredEmptyBody})
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

// handleAgentDeviceMessage processes an incoming message on the global agent
// device. It skips the inbox and dispatches directly to the agent handler which
// resolves the sender's organisation from the phone number.
func (h *Handler) handleAgentDeviceMessage(c *gin.Context, request WhatsAppWebhookEnvelope) {
	var payload whatsAppWebhookPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		httpkit.Error(c, http.StatusBadRequest, whatsAppInvalidMessagePayload, err.Error())
		return
	}
	if payload.IsFromMe {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "outgoing message on agent device"})
		return
	}

	messageAddress := strings.TrimSpace(payload.From)
	if messageAddress == "" {
		messageAddress = strings.TrimSpace(payload.ChatID)
	}
	if isNonDirectWhatsAppAddress(messageAddress) || isNonDirectWhatsAppAddress(payload.ChatID) {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredNonDirectChat})
		return
	}

	body, metadata, err := buildWhatsAppWebhookMessageData(request, payload.Body)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, whatsAppInvalidMessagePayload, err.Error())
		return
	}
	if body == "" {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredEmptyBody})
		return
	}

	if !isNilWhatsAppAgentHandler(h.agentHandler) {
		go h.agentHandler.HandleIncomingMessage(context.WithoutCancel(c.Request.Context()), waagent.CurrentInboundMessage{
			ExternalMessageID: strings.TrimSpace(payload.ID),
			PhoneNumber:       messageAddress,
			DisplayName:       payload.FromName,
			Body:              body,
			Metadata:          metadata,
		})
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func (h *Handler) handleOutgoingWhatsAppMessage(c *gin.Context, orgID uuid.UUID, request WhatsAppWebhookEnvelope, payload whatsAppWebhookPayload) {
	messageAddress := strings.TrimSpace(payload.ChatID)
	if messageAddress == "" {
		messageAddress = strings.TrimSpace(payload.From)
	}
	if isNonDirectWhatsAppAddress(messageAddress) || isNonDirectWhatsAppAddress(payload.ChatID) {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredNonDirectChat})
		return
	}

	body, metadata, err := buildWhatsAppWebhookMessageData(request, payload.Body)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, whatsAppInvalidMessagePayload, err.Error())
		return
	}
	if body == "" {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: whatsAppIgnoredEmptyBody})
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
	receiptType, ok := normalizeWhatsAppReceiptType(payload.ReceiptType)
	messageIDs := payload.messageIDs()
	if !ok || len(messageIDs) == 0 {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "empty receipt payload"})
		return
	}

	applied, err := h.whatsappInbox.ApplyWhatsAppMessageReceipt(c.Request.Context(), orgID, messageIDs, receiptType, parseWhatsAppWebhookTimestamp(payload.Timestamp, request.Timestamp))
	if httpkit.HandleError(c, err) {
		return
	}
	if !applied {
		httpkit.OK(c, WhatsAppWebhookResponse{Status: "ignored", Reason: "unsupported or unmatched receipt"})
		return
	}

	httpkit.OK(c, WhatsAppWebhookResponse{Status: "processed"})
}

func (p whatsAppAckPayload) messageIDs() []string {
	values := make([]string, 0, len(p.IDs)+2)
	values = append(values, p.IDs...)
	if trimmed := strings.TrimSpace(p.ID); trimmed != "" {
		values = append(values, trimmed)
	}
	if trimmed := strings.TrimSpace(p.MessageID); trimmed != "" {
		values = append(values, trimmed)
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
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

func normalizeWhatsAppReceiptType(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "delivered":
		return "delivered", true
	case "read", "read_self":
		return "read", true
	default:
		return "", false
	}
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

func buildWhatsAppWebhookMessageData(request WhatsAppWebhookEnvelope, fallbackBody string) (string, json.RawMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return "", nil, err
	}

	summary := summarizeWhatsAppPayload(payload, fallbackBody)
	envelope := map[string]any{
		"event":   request.Event,
		"payload": payload,
	}
	if trimmed := strings.TrimSpace(request.DeviceID); trimmed != "" {
		envelope["device_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(request.Timestamp); trimmed != "" {
		envelope["timestamp"] = trimmed
	}
	if len(summary.Portal) > 0 {
		envelope["portal"] = summary.Portal
	}

	metadata, err := json.Marshal(envelope)
	if err != nil {
		return "", nil, err
	}

	return summary.Body, metadata, nil
}

type whatsAppPayloadSummary struct {
	Body   string
	Portal map[string]any
}

func summarizeWhatsAppPayload(payload map[string]any, fallbackBody string) whatsAppPayloadSummary {
	summary := whatsAppPayloadSummary{
		Body:   strings.TrimSpace(fallbackBody),
		Portal: map[string]any{},
	}

	if reply := buildWhatsAppReplyPortal(payload); len(reply) > 0 {
		summary.Portal["reply"] = reply
	}
	if viewOnce, ok := boolValue(payload["view_once"]); ok {
		summary.Portal["viewOnce"] = viewOnce
	}
	if forwarded, ok := boolValue(payload["forwarded"]); ok {
		summary.Portal["isForwarded"] = forwarded
	}

	if summary.Body == "" {
		summary.Body = buildWhatsAppStructuredBody(payload, summary.Portal)
	} else {
		buildWhatsAppStructuredBody(payload, summary.Portal)
	}

	if summary.Body != "" && summary.Portal["text"] == nil {
		summary.Portal["text"] = summary.Body
	}
	if summary.Portal["messageType"] == nil {
		summary.Portal["messageType"] = "text"
	}

	return summary
}

func buildWhatsAppReplyPortal(payload map[string]any) map[string]any {
	reply := map[string]any{}
	if repliedToID := strings.TrimSpace(stringValue(payload["replied_to_id"])); repliedToID != "" {
		reply["messageId"] = repliedToID
	}
	if quotedBody := strings.TrimSpace(stringValue(payload["quoted_body"])); quotedBody != "" {
		reply["body"] = quotedBody
	}
	return reply
}

func buildWhatsAppStructuredBody(payload map[string]any, portal map[string]any) string {
	if body := buildWhatsAppPollPortal(payload, portal); body != "" {
		return body
	}

	structuredFields := []struct {
		key         string
		messageType string
		label       string
	}{
		{key: "image", messageType: "image", label: "[Afbeelding]"},
		{key: "video", messageType: "video", label: "[Video]"},
		{key: "audio", messageType: "audio", label: "[Audio]"},
		{key: "document", messageType: "file", label: "[Bestand]"},
		{key: "sticker", messageType: "sticker", label: "[Sticker]"},
		{key: "video_note", messageType: "video_note", label: "[Videonotitie]"},
	}
	for _, field := range structuredFields {
		if body := buildWhatsAppMediaPortal(payload, portal, field.key, field.messageType, field.label); body != "" {
			return body
		}
	}
	if body := buildWhatsAppLocationPortal(payload, portal); body != "" {
		return body
	}
	if body := buildWhatsAppContactPortal(payload, portal); body != "" {
		return body
	}
	return ""
}

func buildWhatsAppMediaPortal(payload map[string]any, portal map[string]any, key string, messageType string, label string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}

	portal["messageType"] = messageType
	attachment := map[string]any{"mediaType": messageType}

	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return ""
		}
		attachment["path"] = trimmed
	case map[string]any:
		if path := strings.TrimSpace(stringValue(typed["path"])); path != "" {
			attachment["path"] = path
		}
		if remoteURL := strings.TrimSpace(stringValue(typed["url"])); remoteURL != "" {
			attachment["remoteUrl"] = remoteURL
		}
		if filename := strings.TrimSpace(stringValue(typed["filename"])); filename != "" {
			attachment["filename"] = filename
		}
		if caption := strings.TrimSpace(stringValue(typed["caption"])); caption != "" {
			portal["caption"] = caption
		}
	default:
		return label
	}

	portal["attachment"] = attachment
	if caption := strings.TrimSpace(stringValue(portal["caption"])); caption != "" {
		return caption
	}
	if filename := strings.TrimSpace(stringValue(attachment["filename"])); filename != "" {
		return fmt.Sprintf("%s %s", label, filename)
	}
	return label
}

func buildWhatsAppContactPortal(payload map[string]any, portal map[string]any) string {
	if body := buildWhatsAppContactsArrayPortal(payload, portal); body != "" {
		return body
	}

	value, ok := payload["contact"]
	if !ok || value == nil {
		return ""
	}
	contactMap, ok := value.(map[string]any)
	if !ok {
		portal["messageType"] = "contact"
		return "[Contact]"
	}

	contact := normalizeWhatsAppContact(contactMap)
	portal["messageType"] = "contact"
	if len(contact) > 0 {
		portal["contact"] = contact
	}
	if name := strings.TrimSpace(stringValue(contact["name"])); name != "" {
		return "[Contact] " + name
	}
	return "[Contact]"
}

func buildWhatsAppContactsArrayPortal(payload map[string]any, portal map[string]any) string {
	value, ok := payload["contacts_array"]
	if !ok || value == nil {
		return ""
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		portal["messageType"] = "contact"
		return "[Contacten]"
	}

	contacts := make([]map[string]any, 0, len(items))
	names := make([]string, 0, len(items))
	for _, item := range items {
		contactMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		contact := normalizeWhatsAppContact(contactMap)
		if len(contact) == 0 {
			continue
		}
		contacts = append(contacts, contact)
		if name := strings.TrimSpace(stringValue(contact["name"])); name != "" {
			names = append(names, name)
		}
	}
	portal["messageType"] = "contact"
	if len(contacts) > 0 {
		portal["contacts"] = contacts
	}
	if len(contacts) == 1 {
		portal["contact"] = contacts[0]
	}
	if len(names) > 0 {
		return "[Contacten] " + strings.Join(names, ", ")
	}
	return "[Contacten]"
}

func normalizeWhatsAppContact(value map[string]any) map[string]any {
	contact := map[string]any{}
	if name := strings.TrimSpace(stringValue(value["displayName"])); name != "" {
		contact["name"] = name
	}
	vcard := strings.TrimSpace(stringValue(value["vcard"]))
	if phoneNumber := extractWhatsAppPhoneFromVCard(vcard); phoneNumber != "" {
		contact["phone"] = phoneNumber
	}
	return contact
}

func extractWhatsAppPhoneFromVCard(vcard string) string {
	if strings.TrimSpace(vcard) == "" {
		return ""
	}
	for _, line := range strings.Split(vcard, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), "TEL") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func buildWhatsAppLocationPortal(payload map[string]any, portal map[string]any) string {
	for _, key := range []string{"location", "live_location"} {
		location, preview, ok := normalizeWhatsAppLocationPayload(payload[key], key == "live_location")
		if !ok {
			continue
		}
		portal["messageType"] = "location"
		if len(location) > 0 {
			portal["location"] = location
		}
		return preview
	}
	return ""
}

func buildWhatsAppPollPortal(payload map[string]any, portal map[string]any) string {
	candidates := collectWhatsAppPollCandidates(payload)
	question := firstWhatsAppCandidateString(candidates, []string{"question", "name", "title"})
	options := firstWhatsAppCandidateStringSlice(candidates, []string{"options", "option_names", "optionNames"})
	selectedOptions := firstWhatsAppCandidateStringSlice(candidates, []string{"selected_options", "selectedOptions", "selected_option_names", "selectedOptionNames", "votes", "vote"})
	maxAnswer := firstWhatsAppCandidateString(candidates, []string{"max_answer", "maxAnswer"})

	if question == "" && len(options) == 0 && len(selectedOptions) == 0 && maxAnswer == "" {
		return ""
	}

	poll := map[string]any{}
	if question != "" {
		poll["question"] = question
	}
	if len(options) > 0 {
		poll["options"] = options
	}
	if len(selectedOptions) > 0 {
		poll["selectedOptions"] = selectedOptions
	}
	if maxAnswer != "" {
		poll["maxAnswer"] = maxAnswer
	}
	portal["messageType"] = "poll"
	portal["poll"] = poll

	return buildWhatsAppPollPreview(question, options, selectedOptions)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		if trimmed == "true" {
			return true, true
		}
		if trimmed == "false" {
			return false, true
		}
	}
	return false, false
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactStringSlice(typed)
	case []any:
		return normalizeAnyStringSlice(typed)
	default:
		trimmed := strings.TrimSpace(stringValue(value))
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
}

func normalizeWhatsAppLocationPayload(value any, live bool) (map[string]any, string, bool) {
	locationMap, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil, "", false
		}
		return nil, "[Locatie]", true
	}

	location := map[string]any{}
	copyLocationValue(location, "latitude", locationMap["degreesLatitude"])
	copyLocationValue(location, "longitude", locationMap["degreesLongitude"])
	copyLocationValue(location, "name", locationMap["name"])
	copyLocationValue(location, "address", locationMap["address"])
	if live {
		location["live"] = true
	}

	return location, buildWhatsAppLocationPreview(location), true
}

func copyLocationValue(target map[string]any, key string, source any) {
	if value := strings.TrimSpace(stringValue(source)); value != "" {
		target[key] = value
	}
}

func buildWhatsAppLocationPreview(location map[string]any) string {
	if name := strings.TrimSpace(stringValue(location["name"])); name != "" {
		return "[Locatie] " + name
	}
	if address := strings.TrimSpace(stringValue(location["address"])); address != "" {
		return "[Locatie] " + address
	}
	return "[Locatie]"
}

func collectWhatsAppPollCandidates(payload map[string]any) []map[string]any {
	candidates := []map[string]any{payload}
	for _, key := range []string{"poll", "poll_update", "pollUpdate"} {
		if nested, ok := payload[key].(map[string]any); ok {
			candidates = append(candidates, nested)
		}
	}
	return candidates
}

func firstWhatsAppCandidateString(candidates []map[string]any, keys []string) string {
	for _, candidate := range candidates {
		for _, key := range keys {
			if value := strings.TrimSpace(stringValue(candidate[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstWhatsAppCandidateStringSlice(candidates []map[string]any, keys []string) []string {
	for _, candidate := range candidates {
		for _, key := range keys {
			if values := stringSliceValue(candidate[key]); len(values) > 0 {
				return values
			}
		}
	}
	return nil
}

func buildWhatsAppPollPreview(question string, options []string, selectedOptions []string) string {
	if len(selectedOptions) > 0 {
		return whatsAppPollPrefix + strings.Join(selectedOptions, ", ")
	}
	if question != "" {
		return whatsAppPollPrefix + question
	}
	if len(options) > 0 {
		return whatsAppPollPrefix + strings.Join(options, ", ")
	}
	return strings.TrimSpace(whatsAppPollPrefix)
}

func compactStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func normalizeAnyStringSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if normalized := normalizeAnyStringValue(item); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func normalizeAnyStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return firstRecordString(typed, []string{"name", "label", "title", "option", "value"})
	default:
		return ""
	}
}

func firstRecordString(record map[string]any, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringValue(record[key])); value != "" {
			return value
		}
	}
	return ""
}
