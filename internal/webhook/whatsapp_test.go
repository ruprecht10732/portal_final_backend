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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const directChatJID = "31612345678@s.whatsapp.net"

type fakeWhatsAppInbox struct {
	seenIDs       map[string]struct{}
	unreadCount   int
	outgoingCount int
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
