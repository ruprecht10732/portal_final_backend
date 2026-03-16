package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"portal_final_backend/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	whatsAppWebhookPath = "/api/v1/webhook/whatsapp"
	jsonContentType     = "application/json"
	contentTypeHeader   = "Content-Type"
	testWebhookSecret   = "super-secret"
	testDeviceID        = "device-123"
	testAccountJID      = "31619330634@s.whatsapp.net"
	signatureHeader     = "X-Hub-Signature-256"
	sharedSecretHeader  = "X-Webhook-Secret"
	expected200Format   = "expected 200, got %d: %s"
	expected401Format   = "expected 401, got %d: %s"
	expectedAgentFlag   = "expected agent device flag"
)

type fakeWebhookAuthRepository struct {
	keysByHash     map[string]APIKey
	orgByDeviceID  map[string]uuid.UUID
	agentDeviceIDs map[string]bool
}

func (f *fakeWebhookAuthRepository) GetByHash(_ context.Context, keyHash string) (APIKey, error) {
	if key, ok := f.keysByHash[keyHash]; ok {
		return key, nil
	}
	return APIKey{}, ErrAPIKeyNotFound
}

func (f *fakeWebhookAuthRepository) GetOrganizationIDByWhatsAppDeviceID(_ context.Context, deviceID string) (uuid.UUID, error) {
	if orgID, ok := f.orgByDeviceID[deviceID]; ok {
		return orgID, nil
	}
	return uuid.UUID{}, ErrWhatsAppDeviceNotFound
}

func (f *fakeWebhookAuthRepository) IsAgentDevice(_ context.Context, deviceID string) (bool, error) {
	return f.agentDeviceIDs[deviceID], nil
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testDeviceID: uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		orgID := c.MustGet("webhookOrgID").(uuid.UUID)
		c.JSON(http.StatusOK, gin.H{"organization_id": orgID.String()})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() == "" {
		t.Fatal("expected response body")
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareRejectsInvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testDeviceID: uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message"}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader("wrong-secret", body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(expected401Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareKeepsAPIKeyFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orgID := uuid.New()
	plaintext := "whk_test_key"
	repo := &fakeWebhookAuthRepository{
		keysByHash: map[string]APIKey{
			HashKey(plaintext): {
				ID:             uuid.New(),
				OrganizationID: orgID,
			},
		},
	}

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		resolved := c.MustGet("webhookOrgID").(uuid.UUID)
		if resolved != orgID {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "wrong organization"})
			return
		}
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath+"?api_key="+plaintext, bytes.NewReader([]byte(`{"event":"message"}`)))
	req.Header.Set(contentTypeHeader, jsonContentType)
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareRejectsMissingSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testDeviceID: uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message"}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(expected401Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareRejectsUnknownDeviceForSignedWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{"other-device": uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(expected401Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhookResolvedByAccountJID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testAccountJID: uuid.New()}}
	body := []byte(`{"device_id":"31619330634@s.whatsapp.net","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhookResolvedByLowercaseJIDCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testAccountJID: uuid.New()}}
	body := []byte(`{"device_id":"31619330634@S.WHATSAPP.NET","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhookResolvedFromBarePhoneCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testAccountJID: uuid.New()}}
	body := []byte(`{"device_id":"31619330634","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSharedSecretHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testDeviceID: uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(sharedSecretHeader, testWebhookSecret)
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSharedSecretQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{testDeviceID: uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath+"?webhook_secret="+testWebhookSecret, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhookForAgentDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{agentDeviceIDs: map[string]bool{testDeviceID: true}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		if !c.GetBool("isAgentDevice") {
			c.JSON(http.StatusInternalServerError, gin.H{"error": expectedAgentFlag})
			return
		}
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhookForAgentDeviceBarePhoneCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentJID := "31612345678@s.whatsapp.net"
	repo := &fakeWebhookAuthRepository{agentDeviceIDs: map[string]bool{agentJID: true}}
	body := []byte(`{"device_id":"31612345678","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		if !c.GetBool("isAgentDevice") {
			c.JSON(http.StatusInternalServerError, gin.H{"error": expectedAgentFlag})
			return
		}
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(expected200Format, recorder.Code, recorder.Body.String())
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareRejectsSignedWebhookForAgentMemberPhoneCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{}
	body := []byte(`{"device_id":"31612345678@s.whatsapp.net","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret, logger.New("development")), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set(signatureHeader, signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(expected401Format, recorder.Code, recorder.Body.String())
	}
}

func signedWebhookHeader(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
