package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type webhookAuthRepository interface {
	GetByHash(ctx context.Context, keyHash string) (APIKey, error)
	GetOrganizationIDByWhatsAppDeviceID(ctx context.Context, deviceID string) (uuid.UUID, error)
}

// APIKeyAuthMiddleware validates the X-Webhook-API-Key header
// and sets the organization context on the gin context.
func APIKeyAuthMiddleware(repo webhookAuthRepository) gin.HandlerFunc {
	return webhookAPIKeyAuthMiddleware(repo, false, true)
}

// WhatsAppAPIKeyAuthMiddleware validates the WhatsApp webhook API key.
// It supports a query-string API key so upstream providers that can only
// configure a URL can still authenticate requests.
func WhatsAppAPIKeyAuthMiddleware(repo webhookAuthRepository, webhookSecret string) gin.HandlerFunc {
	trimmedSecret := strings.TrimSpace(webhookSecret)

	return func(c *gin.Context) {
		if apiKey := webhookAPIKeyFromRequest(c, true); apiKey != "" {
			key, ok := lookupWebhookAPIKey(c, repo, apiKey)
			if !ok {
				return
			}

			c.Set("webhookOrgID", key.OrganizationID)
			c.Set("webhookKeyID", key.ID)
			c.Next()
			return
		}

		if trimmedSecret != "" {
			organizationID, ok := authenticateSignedWhatsAppWebhook(c, repo, trimmedSecret)
			if !ok {
				return
			}

			c.Set("webhookOrgID", organizationID)
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing webhook authentication"})
	}
}

func webhookAPIKeyAuthMiddleware(repo webhookAuthRepository, allowQueryParam bool, validateOrigin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := authenticateWebhookAPIKey(c, repo, allowQueryParam)
		if !ok {
			return
		}

		if validateOrigin && !isWebhookOriginAllowed(c, key.AllowedDomains) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "domain not allowed"})
			return
		}

		// Set organization context for downstream handlers
		c.Set("webhookOrgID", key.OrganizationID)
		c.Set("webhookKeyID", key.ID)
		c.Next()
	}
}

func authenticateWebhookAPIKey(c *gin.Context, repo webhookAuthRepository, allowQueryParam bool) (APIKey, bool) {
	apiKey := webhookAPIKeyFromRequest(c, allowQueryParam)
	if apiKey == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		return APIKey{}, false
	}

	return lookupWebhookAPIKey(c, repo, apiKey)
}

func lookupWebhookAPIKey(c *gin.Context, repo webhookAuthRepository, apiKey string) (APIKey, bool) {
	keyHash := HashKey(apiKey)
	key, err := repo.GetByHash(c.Request.Context(), keyHash)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return APIKey{}, false
	}

	return key, true
}

func authenticateSignedWhatsAppWebhook(c *gin.Context, repo webhookAuthRepository, webhookSecret string) (uuid.UUID, bool) {
	signature := strings.TrimSpace(c.GetHeader("X-Hub-Signature-256"))
	if signature == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing webhook signature"})
		return uuid.UUID{}, false
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return uuid.UUID{}, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	if !isValidWhatsAppWebhookSignature(signature, body, webhookSecret) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return uuid.UUID{}, false
	}

	deviceID, err := extractWhatsAppWebhookDeviceID(body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing whatsapp device_id"})
		return uuid.UUID{}, false
	}

	organizationID, err := repo.GetOrganizationIDByWhatsAppDeviceID(c.Request.Context(), deviceID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unknown whatsapp device"})
		return uuid.UUID{}, false
	}

	return organizationID, true
}

func isValidWhatsAppWebhookSignature(signatureHeader string, body []byte, webhookSecret string) bool {
	parts := strings.SplitN(strings.TrimSpace(signatureHeader), "=", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "sha256") {
		return false
	}

	provided, err := hex.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(provided, expected)
}

func extractWhatsAppWebhookDeviceID(body []byte) (string, error) {
	var payload struct {
		DeviceID string `json:"device_id"`
		Payload  struct {
			DeviceID string `json:"device_id"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	if deviceID := strings.TrimSpace(payload.DeviceID); deviceID != "" {
		return deviceID, nil
	}
	if deviceID := strings.TrimSpace(payload.Payload.DeviceID); deviceID != "" {
		return deviceID, nil
	}

	return "", ErrWhatsAppDeviceNotFound
}

func isWebhookOriginAllowed(c *gin.Context, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}

	// The CORS bypass middleware strips the Origin header and saves
	// it as "webhookOrigin" in the gin context. Fall back to the
	// raw header and Referer for non-browser callers.
	origin := c.GetString("webhookOrigin")
	if origin == "" {
		origin = c.GetHeader("Origin")
	}
	if origin == "" {
		origin = c.GetHeader("Referer")
	}
	return isDomainAllowed(origin, allowedDomains)
}

func webhookAPIKeyFromRequest(c *gin.Context, allowQueryParam bool) string {
	if apiKey := strings.TrimSpace(c.GetHeader("X-Webhook-API-Key")); apiKey != "" {
		return apiKey
	}
	if !allowQueryParam {
		return ""
	}
	return strings.TrimSpace(c.Query("api_key"))
}

// isDomainAllowed checks if the origin matches any of the allowed domains.
// Supports exact match and wildcard subdomains (e.g., "*.example.com").
func isDomainAllowed(origin string, allowedDomains []string) bool {
	if origin == "" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())

	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "*" {
			return true
		}
		if strings.HasPrefix(domain, "*.") {
			// Wildcard subdomain match
			suffix := domain[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) || host == domain[2:] {
				return true
			}
		} else {
			if host == domain {
				return true
			}
		}
	}
	return false
}
