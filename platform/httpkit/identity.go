// Package httpkit provides HTTP utilities including identity abstraction.
package httpkit

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Identity represents the authenticated user's identity.
// This interface abstracts identity extraction from the web framework,
// allowing handlers to access user information without depending on Gin.
type Identity interface {
	// UserID returns the authenticated user's ID.
	UserID() uuid.UUID
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
	roles         []string
	authenticated bool
}

func (i *identity) UserID() uuid.UUID {
	return i.userID
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

	return &identity{
		userID:        uid,
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
	return id
}
