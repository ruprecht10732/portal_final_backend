// Package ports defines consumer-driven interfaces for external dependencies.
// These interfaces are defined in the Leads domain based on what it needs,
// rather than what other domains choose to offer.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// UserInfo represents the minimal user data the leads domain needs.
type UserInfo struct {
	ID    uuid.UUID
	Email string
	Roles []string
}

// UserProvider provides user information needed by the leads domain.
// This interface is defined here (consumer-driven) rather than in the auth domain.
// The auth domain's repository or service can implement this interface.
type UserProvider interface {
	// GetUserByID returns basic user info. Returns error if user not found.
	GetUserByID(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}

// UserExistenceChecker verifies if users exist without exposing full user data.
// Useful for validating assignee IDs without tight coupling to auth internals.
type UserExistenceChecker interface {
	// UserExists returns true if a user with the given ID exists.
	UserExists(ctx context.Context, userID uuid.UUID) (bool, error)
}

// UserLister provides a list of users for assignment dropdowns.
type UserLister interface {
	// ListAssignableUsers returns users that can be assigned to leads.
	// Implementation may filter by role (e.g., agents, scouts).
	ListAssignableUsers(ctx context.Context) ([]UserInfo, error)
}
