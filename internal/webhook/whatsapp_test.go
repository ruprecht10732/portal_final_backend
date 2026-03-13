package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portal_final_backend/internal/waagent"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const directChatJID = "31612345678@s.whatsapp.net"
const mediaExamplePath = "statics/media/example.jpeg"
const errUnmarshalMetadataFmt = "unmarshal metadata: %v"

type fakeWhatsAppInbox struct {
	seenIDs       map[string]struct{}
	unreadCount   int
	outgoingCount int
	lastIncoming  *IncomingWhatsAppMessage
	lastOutgoing  *OutgoingWhatsAppMessage
	receiptTypes  map[string]string
	mutations     []WhatsAppMessageMutation
}

func (f *fakeWhatsAppInbox) ReceiveIncomingWhatsAppMessage(_ context.Context, message IncomingWhatsAppMessage) (bool, error) {
	if f.seenIDs == nil {
		f.seenIDs = map[string]struct{}{}
	}
	if message.ExternalMessageID != nil {
		if _, exists := f.seenIDs[*message.ExternalMessageID]; exists {
			return false, nil
		}
		f.seenIDs[*message.ExternalMessageID] = struct{}{}
	}
	f.unreadCount++
	copy := message
	f.lastIncoming = &copy
	return true, nil
}

func (f *fakeWhatsAppInbox) SyncOutgoingWhatsAppMessage(_ context.Context, message OutgoingWhatsAppMessage) (bool, error) {
	if f.seenIDs == nil {
		f.seenIDs = map[string]struct{}{}
	}
	if message.ExternalMessageID != nil {
		if _, exists := f.seenIDs[*message.ExternalMessageID]; exists {
			return false, nil
		}
		f.seenIDs[*message.ExternalMessageID] = struct{}{}
	}
	f.outgoingCount++
	copy := message
	f.lastOutgoing = &copy
	return true, nil
}

func (f *fakeWhatsAppInbox) ApplyWhatsAppMessageReceipt(_ context.Context, _ uuid.UUID, externalMessageIDs []string, receiptType string, _ *time.Time) (bool, error) {
	if len(externalMessageIDs) == 0 {
		return false, nil
	}
	if f.receiptTypes == nil {
		f.receiptTypes = map[string]string{}
	}
	for _, id := range externalMessageIDs {
		f.receiptTypes[id] = receiptType
	}
	return receiptType == "delivered" || receiptType == "read", nil
}

func (f *fakeWhatsAppInbox) ApplyWhatsAppMessageMutation(_ context.Context, message WhatsAppMessageMutation) (bool, error) {
	f.mutations = append(f.mutations, message)
	return strings.TrimSpace(message.TargetExternalMessageID) != "", nil
}

type fakeWhatsAppAgentHandler struct {
	lastInbound *waagent.CurrentInboundMessage
	called      chan struct{}
}

func (f *fakeWhatsAppAgentHandler) HandleIncomingMessage(_ context.Context, inbound waagent.CurrentInboundMessage) {
	copy := inbound
	f.lastInbound = &copy
	if f.called != nil {
		select {
		case f.called <- struct{}{}:
		default:
		}
	}
}

func TestHandleWhatsAppWebhookDedupesIncomingMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message",
		"timestamp": "2026-03-09T10:00:00Z",
		"payload": map[string]any{
			"id":         "MSG-1",
			"from":       directChatJID,
			"chat_id":    directChatJID,
			"from_name":  "Robin",
			"is_from_me": false,
			"body":       "Hallo",
		},
	}

	first := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200 for first inbound webhook, got %d", first.Code)
	}
	assertWebhookStatus(t, first.Body.Bytes(), "processed")

	second := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if second.Code != http.StatusOK {
		t.Fatalf("expected 200 for duplicate inbound webhook, got %d", second.Code)
	}
	assertWebhookStatus(t, second.Body.Bytes(), "duplicate")
	if ingester.unreadCount != 1 {
		t.Fatalf("expected duplicate webhook to keep unread count at 1, got %d", ingester.unreadCount)
	}
}

func TestHandleWhatsAppWebhookSyncsOutgoingDeviceMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message",
		"timestamp": "2026-03-09T10:05:00Z",
		"payload": map[string]any{
			"id":         "MSG-OUT-1",
			"from":       "31699999999@s.whatsapp.net",
			"chat_id":    directChatJID,
			"from_name":  "Robin",
			"is_from_me": true,
			"body":       "Follow-up vanaf telefoon",
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for outgoing device webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if ingester.outgoingCount != 1 {
		t.Fatalf("expected 1 synced outgoing message, got %d", ingester.outgoingCount)
	}
	if ingester.lastOutgoing == nil {
		t.Fatal("expected outgoing payload to be captured")
	}
	if ingester.lastOutgoing.PhoneNumber != directChatJID {
		t.Fatalf("expected outgoing sync to target chat_id, got %q", ingester.lastOutgoing.PhoneNumber)
	}
	if ingester.unreadCount != 0 {
		t.Fatalf("expected outgoing sync not to change unread count, got %d", ingester.unreadCount)
	}
}

func TestHandleWhatsAppWebhookAppliesReadReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message.ack",
		"timestamp": "2026-03-09T11:00:00Z",
		"payload": map[string]any{
			"ids":          []string{"OUT-1", "OUT-2"},
			"receipt_type": "read",
			"timestamp":    "2026-03-09T11:00:00Z",
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for receipt webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if ingester.receiptTypes["OUT-1"] != "read" || ingester.receiptTypes["OUT-2"] != "read" {
		t.Fatalf("expected receipt type to be recorded for all message ids, got %#v", ingester.receiptTypes)
	}
	if ingester.unreadCount != 0 {
		t.Fatalf("expected receipts not to affect unread count, got %d", ingester.unreadCount)
	}
}

func TestHandleWhatsAppWebhookNormalizesReadSelfReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message.ack",
		"timestamp": "2026-03-10T09:00:00Z",
		"payload": map[string]any{
			"id":           "OUT-SELF-1",
			"receipt_type": "read-self",
			"timestamp":    "2026-03-10T09:00:00Z",
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for read-self receipt webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if ingester.receiptTypes["OUT-SELF-1"] != "read" {
		t.Fatalf("expected read-self receipt to normalize to read, got %#v", ingester.receiptTypes)
	}
}

func TestHandleWhatsAppWebhookAppliesEditedMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message.edited",
		"timestamp": "2026-03-09T11:05:00Z",
		"payload": map[string]any{
			"id":                  "EDIT-EVT-1",
			"chat_id":             directChatJID,
			"from":                directChatJID,
			"from_name":           "Robin",
			"timestamp":           "2026-03-09T11:05:00Z",
			"is_from_me":          false,
			"original_message_id": "MSG-1",
			"body":                "Bijgewerkte tekst",
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for edited webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if len(ingester.mutations) != 1 {
		t.Fatalf("expected exactly one mutation, got %d", len(ingester.mutations))
	}
	if ingester.mutations[0].TargetExternalMessageID != "MSG-1" {
		t.Fatalf("expected original message id to be targeted, got %q", ingester.mutations[0].TargetExternalMessageID)
	}
	if ingester.mutations[0].Body == nil || *ingester.mutations[0].Body != "Bijgewerkte tekst" {
		t.Fatalf("expected updated body to be forwarded, got %#v", ingester.mutations[0].Body)
	}
}

func TestHandleWhatsAppWebhookAppliesReactionMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message.reaction",
		"timestamp": "2026-03-09T11:10:00Z",
		"payload": map[string]any{
			"id":                 "REACTION-1",
			"chat_id":            directChatJID,
			"from":               directChatJID,
			"from_name":          "Robin",
			"timestamp":          "2026-03-09T11:10:00Z",
			"is_from_me":         false,
			"reaction":           "👍",
			"reacted_message_id": "MSG-1",
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for reaction webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if len(ingester.mutations) != 1 {
		t.Fatalf("expected exactly one mutation, got %d", len(ingester.mutations))
	}
	if ingester.mutations[0].Reaction == nil || *ingester.mutations[0].Reaction != "👍" {
		t.Fatalf("expected reaction to be forwarded, got %#v", ingester.mutations[0].Reaction)
	}
	if ingester.mutations[0].TargetExternalMessageID != "MSG-1" {
		t.Fatalf("expected reacted message id to be targeted, got %q", ingester.mutations[0].TargetExternalMessageID)
	}
}

func TestHandleWhatsAppWebhookBuildsMediaPortalMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message",
		"timestamp": "2026-03-10T10:00:00Z",
		"payload": map[string]any{
			"id":            "MEDIA-1",
			"from":          directChatJID,
			"chat_id":       directChatJID,
			"from_name":     "Robin",
			"is_from_me":    false,
			"replied_to_id": "MSG-ROOT",
			"quoted_body":   "Origineel bericht",
			"view_once":     true,
			"image": map[string]any{
				"path":    mediaExamplePath,
				"caption": "Kijk hiernaar",
			},
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for media webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if ingester.lastIncoming == nil {
		t.Fatal("expected incoming payload to be captured")
	}
	if ingester.lastIncoming.Body != "Kijk hiernaar" {
		t.Fatalf("expected caption to become body preview, got %q", ingester.lastIncoming.Body)
	}

	var metadata map[string]any
	if err := json.Unmarshal(ingester.lastIncoming.Metadata, &metadata); err != nil {
		t.Fatalf(errUnmarshalMetadataFmt, err)
	}
	portal, ok := metadata["portal"].(map[string]any)
	if !ok {
		t.Fatal("expected portal metadata to be present")
	}
	if portal["messageType"] != "image" {
		t.Fatalf("expected image portal type, got %#v", portal["messageType"])
	}
	attachment, ok := portal["attachment"].(map[string]any)
	if !ok || attachment["path"] != mediaExamplePath {
		t.Fatalf("expected image attachment path, got %#v", portal["attachment"])
	}
	reply, ok := portal["reply"].(map[string]any)
	if !ok || reply["body"] != "Origineel bericht" {
		t.Fatalf("expected reply metadata, got %#v", portal["reply"])
	}
	if portal["viewOnce"] != true {
		t.Fatalf("expected viewOnce flag, got %#v", portal["viewOnce"])
	}
}

func TestHandleWhatsAppWebhookBuildsContactsArrayPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingester := &fakeWhatsAppInbox{}
	handler := NewHandler(nil, nil, nil, ingester)
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message",
		"timestamp": "2026-03-10T11:00:00Z",
		"payload": map[string]any{
			"id":         "CONTACTS-1",
			"from":       directChatJID,
			"chat_id":    directChatJID,
			"from_name":  "Robin",
			"is_from_me": false,
			"contacts_array": []any{
				map[string]any{
					"displayName": "Alice",
					"vcard":       "BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nTEL;type=Mobile:+31 6 11111111\nEND:VCARD",
				},
				map[string]any{
					"displayName": "Bob",
					"vcard":       "BEGIN:VCARD\nVERSION:3.0\nFN:Bob\nTEL;type=Mobile:+31 6 22222222\nEND:VCARD",
				},
			},
		},
	}

	response := executeWhatsAppWebhookRequest(t, handler, orgID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for contacts webhook, got %d", response.Code)
	}
	assertWebhookStatus(t, response.Body.Bytes(), "processed")
	if ingester.lastIncoming == nil {
		t.Fatal("expected incoming payload to be captured")
	}
	if ingester.lastIncoming.Body != "[Contacten] Alice, Bob" {
		t.Fatalf("expected contacts preview body, got %q", ingester.lastIncoming.Body)
	}

	var metadata map[string]any
	if err := json.Unmarshal(ingester.lastIncoming.Metadata, &metadata); err != nil {
		t.Fatalf(errUnmarshalMetadataFmt, err)
	}
	portal, ok := metadata["portal"].(map[string]any)
	if !ok {
		t.Fatal("expected portal metadata to be present")
	}
	contacts, ok := portal["contacts"].([]any)
	if !ok || len(contacts) != 2 {
		t.Fatalf("expected 2 contacts in portal metadata, got %#v", portal["contacts"])
	}
}

func TestHandleWhatsAppWebhookAgentDeviceForwardsInboundMediaContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentHandler := &fakeWhatsAppAgentHandler{called: make(chan struct{}, 1)}
	handler := NewHandler(nil, nil, nil, nil)
	handler.agentHandler = agentHandler
	orgID := uuid.New()

	body := map[string]any{
		"event":     "message",
		"device_id": "DEVICE-AGENT",
		"timestamp": "2026-03-13T12:00:00Z",
		"payload": map[string]any{
			"id":         "MEDIA-AGENT-1",
			"from":       directChatJID,
			"chat_id":    directChatJID,
			"from_name":  "Robin",
			"is_from_me": false,
			"image": map[string]any{
				"path":     mediaExamplePath,
				"filename": "example.jpeg",
				"caption":  "Nieuwe foto",
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhook/whatsapp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("webhookOrgID", orgID)
	ctx.Set("isAgentDevice", true)

	handler.HandleWhatsAppWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	select {
	case <-agentHandler.called:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected agent handler to receive inbound message")
	}
	if agentHandler.lastInbound == nil {
		t.Fatal("expected inbound payload to be stored by fake handler")
	}
	if agentHandler.lastInbound.ExternalMessageID != "MEDIA-AGENT-1" {
		t.Fatalf("expected external message id to be forwarded, got %q", agentHandler.lastInbound.ExternalMessageID)
	}
	if agentHandler.lastInbound.Body != "Nieuwe foto" {
		t.Fatalf("expected image caption as body, got %q", agentHandler.lastInbound.Body)
	}
	var metadata map[string]any
	if err := json.Unmarshal(agentHandler.lastInbound.Metadata, &metadata); err != nil {
		t.Fatalf(errUnmarshalMetadataFmt, err)
	}
	if metadata["device_id"] != "DEVICE-AGENT" {
		t.Fatalf("expected device_id in metadata, got %#v", metadata["device_id"])
	}
	portal, ok := metadata["portal"].(map[string]any)
	if !ok || portal["messageType"] != "image" {
		t.Fatalf("expected image portal metadata, got %#v", metadata["portal"])
	}
}

func executeWhatsAppWebhookRequest(t *testing.T, handler *Handler, orgID uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhook/whatsapp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("webhookOrgID", orgID)
	handler.HandleWhatsAppWebhook(ctx)
	return recorder
}

func assertWebhookStatus(t *testing.T, body []byte, expected string) {
	t.Helper()
	var response WhatsAppWebhookResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Status != expected {
		t.Fatalf("expected response status %q, got %q", expected, response.Status)
	}
}
