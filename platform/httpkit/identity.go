// Package httpkit provides HTTP utilities including identity abstraction.
package httpkit

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Identity represents the authenticated user's identity.
// This interface abstracts identity extraction from the web framework,
// allowing handlers to access user information without depending on Gin.
type Identity interface {
	// UserID returns the authenticated user's ID.
	UserID() uuid.UUID
	// TenantID returns the organization ID associated with the user.
	TenantID() *uuid.UUID
	// Roles returns the user's assigned roles.
	Roles() []string
	// HasRole checks if the user has a specific role.
	HasRole(role string) bool
	// IsAuthenticated returns true if the user is authenticated.
	IsAuthenticated() bool
}

// identity is the concrete implementation of Identity.
type identity struct {
	userID        uuid.UUID
	tenantID      *uuid.UUID
	roles         []string
	authenticated bool
}

func (i *identity) UserID() uuid.UUID {
	return i.userID
}

func (i *identity) TenantID() *uuid.UUID {
	return i.tenantID
}

func (i *identity) Roles() []string {
	return i.roles
}

func (i *identity) HasRole(role string) bool {
	for _, r := range i.roles {
		if r == role {
			return true
		}
	}
	return false
}

func (i *identity) IsAuthenticated() bool {
	return i.authenticated
}

// GetIdentity extracts the Identity from a Gin context.
// Returns an unauthenticated identity if user info is not present.
func GetIdentity(c *gin.Context) Identity {
	userID, userOK := c.Get(ContextUserIDKey)
	roles, rolesOK := c.Get(ContextRolesKey)
	tenantID, tenantOK := c.Get(ContextTenantIDKey)

	if !userOK {
		return &identity{authenticated: false}
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		return &identity{authenticated: false}
	}

	var roleList []string
	if rolesOK {
		roleList, _ = roles.([]string)
	}

	var tenantUUID *uuid.UUID
	if tenantOK {
		if rawTenantID, ok := tenantID.(uuid.UUID); ok {
			tenantUUID = &rawTenantID
		}
	}

	return &identity{
		userID:        uid,
		tenantID:      tenantUUID,
		roles:         roleList,
		authenticated: true,
	}
}

// MustGetIdentity extracts the Identity from a Gin context.
// If the user is not authenticated, it aborts with 401 Unauthorized and returns nil.
func MustGetIdentity(c *gin.Context) Identity {
	id := GetIdentity(c)
	if !id.IsAuthenticated() {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil
	}
	if id.TenantID() == nil {
		if !isOnboardingAllowedPath(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "organization required"})
			return nil
		}
	}
	return id
}

func isOnboardingAllowedPath(c *gin.Context) bool {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	allowed := map[string]bool{
		"/api/v1/users/me":            true,
		"/api/v1/users/me/onboarding": true,
		"/api/v1/users/me/password":   true,
	}
	return allowed[path]
}
