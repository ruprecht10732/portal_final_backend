// Package httpkit provides HTTP middleware infrastructure.
// This is part of the platform layer and contains no business logic.
package httpkit

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

const (
	// ContextUserIDKey is the gin context key for the authenticated user ID.
	ContextUserIDKey = "userID"
	// ContextRolesKey is the gin context key for the user's roles.
	ContextRolesKey = "roles"
	// ContextTenantIDKey is the gin context key for the tenant (organization) ID.
	ContextTenantIDKey = "tenantID"

	errMissingToken = "missing token"
	errInvalidToken = "invalid token"
)

// RequestLogger logs HTTP requests with timing.
func RequestLogger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		log.HTTPRequest(c.Request.Method, path, status, float64(latency.Milliseconds()), clientIP)
	}
}

// SecurityHeaders adds security headers to responses.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Only add HSTS in production
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

// IPRateLimiter manages per-IP rate limiters.
type IPRateLimiter struct {
	limiters sync.Map
	rate     rate.Limit
	burst    int
	log      *logger.Logger
}

// NewIPRateLimiter creates a new IP-based rate limiter.
func NewIPRateLimiter(r rate.Limit, burst int, log *logger.Logger) *IPRateLimiter {
	return &IPRateLimiter{
		rate:  r,
		burst: burst,
		log:   log,
	}
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	limiter, exists := i.limiters.Load(ip)
	if !exists {
		newLimiter := rate.NewLimiter(i.rate, i.burst)
		i.limiters.Store(ip, newLimiter)
		return newLimiter
	}
	return limiter.(*rate.Limiter)
}

// RateLimit returns a middleware that rate limits by IP.
func (i *IPRateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := i.getLimiter(ip)

		if !limiter.Allow() {
			if i.log != nil {
				i.log.RateLimitExceeded(ip, c.Request.URL.Path)
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

// AuthRateLimiter is a stricter rate limiter for auth endpoints.
type AuthRateLimiter struct {
	*IPRateLimiter
}

// NewAuthRateLimiter creates a rate limiter for authentication endpoints
// with stricter limits (e.g., 5 requests per minute).
func NewAuthRateLimiter(log *logger.Logger) *AuthRateLimiter {
	return &AuthRateLimiter{
		IPRateLimiter: NewIPRateLimiter(rate.Limit(5.0/60.0), 5, log), // 5 requests per minute, burst of 5
	}
}

// AuthRequired returns middleware that validates JWT access tokens.
// Supports token via Authorization header (Bearer) or query param (for SSE).
func AuthRequired(cfg config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			// Fallback to query param for SSE connections
			rawToken = c.Query("token")
			if rawToken == "" {
				abortUnauthorized(c, errMissingToken)
				return
			}
		}

		claims, err := parseAccessClaims(rawToken, cfg)
		if err != nil {
			abortUnauthorized(c, errInvalidToken)
			return
		}

		userID, err := parseUserID(claims)
		if err != nil {
			abortUnauthorized(c, errInvalidToken)
			return
		}

		roles := extractRoles(claims["roles"])
		c.Set(ContextUserIDKey, userID)
		c.Set(ContextRolesKey, roles)

		if tenantID, err := parseTenantID(claims); err != nil {
			abortUnauthorized(c, errInvalidToken)
			return
		} else if tenantID != nil {
			c.Set(ContextTenantIDKey, *tenantID)
		}
		c.Next()
	}
}

// RequireRole returns middleware that checks if the user has the specified role.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, ok := c.Get(ContextRolesKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		roleList, ok := roles.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		for _, item := range roleList {
			if item == role {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func extractRoles(value interface{}) []string {
	roles := make([]string, 0)
	if value == nil {
		return roles
	}

	switch typed := value.(type) {
	case []string:
		return append(roles, typed...)
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				roles = append(roles, text)
			}
		}
	}

	return roles
}

func extractBearerToken(authHeader string) (string, bool) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}

	rawToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if rawToken == "" {
		return "", false
	}

	return rawToken, true
}

func parseAccessClaims(rawToken string, cfg config.JWTConfig) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(cfg.GetJWTAccessSecret()), nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New(errInvalidToken)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New(errInvalidToken)
	}

	if tokenType, _ := claims["type"].(string); tokenType != "access" {
		return nil, errors.New(errInvalidToken)
	}

	return claims, nil
}

func parseUserID(claims jwt.MapClaims) (uuid.UUID, error) {
	userIDRaw, _ := claims["sub"].(string)
	return uuid.Parse(userIDRaw)
}

func parseTenantID(claims jwt.MapClaims) (*uuid.UUID, error) {
	value, ok := claims["tenant_id"].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
