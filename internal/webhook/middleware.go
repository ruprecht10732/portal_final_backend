package webhook

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthMiddleware validates the X-Webhook-API-Key header
// and sets the organization context on the gin context.
func APIKeyAuthMiddleware(repo *Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-Webhook-API-Key")
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
			return
		}

		keyHash := HashKey(apiKey)
		key, err := repo.GetByHash(c.Request.Context(), keyHash)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			return
		}

		// Domain validation (if allowed_domains is configured)
		if len(key.AllowedDomains) > 0 {
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
			if !isDomainAllowed(origin, key.AllowedDomains) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "domain not allowed"})
				return
			}
		}

		// Set organization context for downstream handlers
		c.Set("webhookOrgID", key.OrganizationID)
		c.Set("webhookKeyID", key.ID)
		c.Next()
	}
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
