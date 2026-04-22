package whatsappagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const expectedDeviceHandlerStatus200Fmt = "expected 200, got %d"

type deviceHandlerTestQueries struct {
	config         whatsappagentdb.RacWhatsappAgentConfig
	getErr         error
	upsertResult   whatsappagentdb.RacWhatsappAgentConfig
	upsertErr      error
	deleteErr      error
	lastUpsert     whatsappagentdb.UpsertAgentConfigParams
	upsertCalls    int
	deleteCalls    int
	getConfigCalls int
}

func (q *deviceHandlerTestQueries) GetAgentConfig(context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error) {
	q.getConfigCalls++
	if q.getErr != nil {
		return whatsappagentdb.RacWhatsappAgentConfig{}, q.getErr
	}
	return q.config, nil
}

func (q *deviceHandlerTestQueries) UpsertAgentConfig(_ context.Context, arg whatsappagentdb.UpsertAgentConfigParams) (whatsappagentdb.RacWhatsappAgentConfig, error) {
	q.upsertCalls++
	q.lastUpsert = arg
	if q.upsertErr != nil {
		return whatsappagentdb.RacWhatsappAgentConfig{}, q.upsertErr
	}
	if q.upsertResult.DeviceID == "" {
		q.upsertResult = whatsappagentdb.RacWhatsappAgentConfig{
			ID:         pgtype.UUID{Bytes: uuid.New(), Valid: true},
			DeviceID:   arg.DeviceID,
			AccountJid: arg.AccountJid,
			CreatedAt:  pgtype.Timestamptz{Time: time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC), Valid: true},
		}
	}
	return q.upsertResult, nil
}

func (q *deviceHandlerTestQueries) DeleteAgentConfig(context.Context) error {
	q.deleteCalls++
	return q.deleteErr
}

type deviceHandlerTestTransport struct {
	createErr           error
	qr                  []byte
	qrErr               error
	status              *whatsapp.DeviceStatusResponse
	statusErr           error
	info                *whatsapp.DeviceInfoResponse
	infoErr             error
	reconnectErr        error
	deleteErr           error
	lastCreatedDeviceID string
	lastQRDeviceID      string
	lastStatusDeviceID  string
	lastInfoDeviceID    string
	lastReconnectID     string
	lastDeleteID        string
}

func (t *deviceHandlerTestTransport) CreateDevice(_ context.Context, deviceID string) error {
	t.lastCreatedDeviceID = deviceID
	return t.createErr
}

func (t *deviceHandlerTestTransport) GetLoginQR(_ context.Context, deviceID string) ([]byte, error) {
	t.lastQRDeviceID = deviceID
	return t.qr, t.qrErr
}

func (t *deviceHandlerTestTransport) GetDeviceStatus(_ context.Context, deviceID string) (*whatsapp.DeviceStatusResponse, error) {
	t.lastStatusDeviceID = deviceID
	return t.status, t.statusErr
}

func (t *deviceHandlerTestTransport) GetDeviceInfo(_ context.Context, deviceID string) (*whatsapp.DeviceInfoResponse, error) {
	t.lastInfoDeviceID = deviceID
	return t.info, t.infoErr
}

func (t *deviceHandlerTestTransport) ReconnectDevice(_ context.Context, deviceID string) error {
	t.lastReconnectID = deviceID
	return t.reconnectErr
}

func (t *deviceHandlerTestTransport) DeleteDevice(_ context.Context, deviceID string) error {
	t.lastDeleteID = deviceID
	return t.deleteErr
}

func newDeviceHandlerTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx, recorder
}

func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return payload
}

func TestDeviceHandlerGetWebhookConfigReturnsExpectedPayload(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &DeviceHandler{webhookSecret: " secret-value "}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodGet, "/webhook-config")

	handler.GetWebhookConfig(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expectedDeviceHandlerStatus200Fmt, recorder.Code)
	}
	payload := decodeJSONBody(t, recorder)
	if payload["sharedSecret"] != "secret-value" {
		t.Fatalf("expected trimmed secret, got %#v", payload["sharedSecret"])
	}
}

func TestDeviceHandlerRegisterCreatesDeviceAndPersistsConfig(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	queries := &deviceHandlerTestQueries{}
	transport := &deviceHandlerTestTransport{}
	handler := &DeviceHandler{queries: queries, waClient: transport}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodPost, "/register")

	handler.Register(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if transport.lastCreatedDeviceID == "" {
		t.Fatal("expected device to be created")
	}
	if queries.deleteCalls != 1 {
		t.Fatalf("expected one delete before upsert, got %d", queries.deleteCalls)
	}
	if queries.upsertCalls != 1 {
		t.Fatalf("expected one upsert, got %d", queries.upsertCalls)
	}
	if queries.lastUpsert.DeviceID != transport.lastCreatedDeviceID {
		t.Fatalf("expected persisted device id %q, got %q", transport.lastCreatedDeviceID, queries.lastUpsert.DeviceID)
	}
}

func TestDeviceHandlerGetQRReturnsNotFoundWithoutConfig(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &DeviceHandler{queries: &deviceHandlerTestQueries{getErr: errors.New("missing")}, waClient: &deviceHandlerTestTransport{}}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodGet, "/qr")

	handler.GetQR(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestDeviceHandlerGetStatusReturnsUnregisteredWhenConfigMissing(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &DeviceHandler{queries: &deviceHandlerTestQueries{getErr: errors.New("missing")}, waClient: &deviceHandlerTestTransport{}}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodGet, "/status")

	handler.GetStatus(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expectedDeviceHandlerStatus200Fmt, recorder.Code)
	}
	payload := decodeJSONBody(t, recorder)
	if payload["state"] != "UNREGISTERED" {
		t.Fatalf("expected UNREGISTERED, got %#v", payload["state"])
	}
}

func TestDeviceHandlerGetStatusPersistsAccountJIDWhenLoggedIn(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	queries := &deviceHandlerTestQueries{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}
	transport := &deviceHandlerTestTransport{
		status: &whatsapp.DeviceStatusResponse{DeviceID: testSenderDeviceID, IsLoggedIn: true},
		info:   &whatsapp.DeviceInfoResponse{JID: " 12345@s.whatsapp.net "},
	}
	handler := &DeviceHandler{queries: queries, waClient: transport}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodGet, "/status")

	handler.GetStatus(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeJSONBody(t, recorder)
	if payload["state"] != "CONNECTED" {
		t.Fatalf("expected CONNECTED, got %#v", payload["state"])
	}
	if payload["accountJid"] != "12345@s.whatsapp.net" {
		t.Fatalf("expected trimmed jid, got %#v", payload["accountJid"])
	}
	if queries.upsertCalls != 1 {
		t.Fatalf("expected one jid persist upsert, got %d", queries.upsertCalls)
	}
	if !queries.lastUpsert.AccountJid.Valid || queries.lastUpsert.AccountJid.String != "12345@s.whatsapp.net" {
		t.Fatalf("expected jid upsert, got %#v", queries.lastUpsert.AccountJid)
	}
}

func TestDeviceHandlerReconnectUsesConfiguredDevice(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	transport := &deviceHandlerTestTransport{}
	handler := &DeviceHandler{queries: &deviceHandlerTestQueries{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, waClient: transport}
	ctx, recorder := newDeviceHandlerTestContext(http.MethodPost, "/reconnect")

	handler.Reconnect(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expectedDeviceHandlerStatus200Fmt, recorder.Code)
	}
	if transport.lastReconnectID != testSenderDeviceID {
		t.Fatalf("expected reconnect for %q, got %q", testSenderDeviceID, transport.lastReconnectID)
	}
}

func TestDeviceHandlerDisconnectClearsConfig(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	queries := &deviceHandlerTestQueries{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}
	transport := &deviceHandlerTestTransport{}
	handler := &DeviceHandler{queries: queries, waClient: transport}
	ctx, _ := newDeviceHandlerTestContext(http.MethodDelete, "/")

	handler.Disconnect(ctx)

	if ctx.Writer.Status() != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", ctx.Writer.Status())
	}
	if transport.lastDeleteID != testSenderDeviceID {
		t.Fatalf("expected delete for %q, got %q", testSenderDeviceID, transport.lastDeleteID)
	}
	if queries.deleteCalls != 1 {
		t.Fatalf("expected one config delete, got %d", queries.deleteCalls)
	}
}
