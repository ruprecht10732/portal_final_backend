// Package ports defines consumer-driven interfaces for external dependencies.
// These interfaces are defined in the Leads domain based on what it needs,
// rather than what other domains choose to offer.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// UserInfo represents the minimal user data the RAC_leads domain needs.
type UserInfo struct {
	ID    uuid.UUID
	Email string
	Roles []string
}

// UserProvider provides user information needed by the RAC_leads domain.
// This interface is defined here (consumer-driven) rather than in the auth domain.
// The auth domain's repository or service can implement this interface.
type UserProvider interface {
	// GetUserByID returns basic user info. Returns error if user not found.
	GetUserByID(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}
