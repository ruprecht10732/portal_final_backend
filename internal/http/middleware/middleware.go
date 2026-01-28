package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"portal_final_backend/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	ContextUserIDKey = "userID"
	ContextRolesKey  = "roles"
	errMissingToken  = "missing token"
	errInvalidToken  = "invalid token"
)

func RequestTimer() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		_ = time.Since(start)
	}
}

func AuthRequired(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c, errMissingToken)
			return
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
		c.Next()
	}
}

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

func parseAccessClaims(rawToken string, cfg *config.Config) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(cfg.JWTAccessSecret), nil
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

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
