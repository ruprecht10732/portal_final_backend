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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	whatsAppWebhookPath = "/api/v1/webhook/whatsapp"
	jsonContentType     = "application/json"
	contentTypeHeader   = "Content-Type"
	testWebhookSecret   = "super-secret"
)

type fakeWebhookAuthRepository struct {
	keysByHash    map[string]APIKey
	orgByDeviceID map[string]uuid.UUID
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

func TestWhatsAppAPIKeyAuthMiddlewareAcceptsSignedWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{"device-123": uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message","payload":{"id":"MSG-1"}}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret), func(c *gin.Context) {
		orgID := c.MustGet("webhookOrgID").(uuid.UUID)
		c.JSON(http.StatusOK, gin.H{"organization_id": orgID.String()})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set("X-Hub-Signature-256", signedWebhookHeader(testWebhookSecret, body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() == "" {
		t.Fatal("expected response body")
	}
}

func TestWhatsAppAPIKeyAuthMiddlewareRejectsInvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeWebhookAuthRepository{orgByDeviceID: map[string]uuid.UUID{"device-123": uuid.New()}}
	body := []byte(`{"device_id":"device-123","event":"message"}`)

	engine := gin.New()
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, whatsAppWebhookPath, bytes.NewReader(body))
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.Header.Set("X-Hub-Signature-256", signedWebhookHeader("wrong-secret", body))
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
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
	engine.POST(whatsAppWebhookPath, WhatsAppAPIKeyAuthMiddleware(repo, testWebhookSecret), func(c *gin.Context) {
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
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func signedWebhookHeader(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
