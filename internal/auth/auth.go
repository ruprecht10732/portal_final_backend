// Package auth provides authentication and authorization functionality.
// This file defines the public API of the auth bounded context.
// Only types and interfaces defined here should be imported by other domains.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Profile represents user information that can be shared with other domains.
type Profile struct {
	ID            uuid.UUID
	Email         string
	EmailVerified bool
	FirstName     *string
	LastName      *string
	PreferredLang string
	Roles         []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UserSummary represents minimal user information for listing purposes.
type UserSummary struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

// Service defines the public interface for authentication operations.
// Other domains should depend on this interface, not on concrete implementations.
type Service interface {
	// GetMe returns the profile of the user with the given ID.
	GetMe(ctx context.Context, userID uuid.UUID) (Profile, error)
	// ListUsers returns a list of all users (for admin purposes).
	ListUsers(ctx context.Context) ([]UserSummary, error)
}

// UserProvider is an interface that other domains can use to get user information.
// This abstracts authentication details from other bounded contexts.
type UserProvider interface {
	// GetUserByID returns basic user information needed by other domains.
	GetUserByID(ctx context.Context, userID uuid.UUID) (Profile, error)
	// GetUsersByIDs returns user information for multiple users at once.
	GetUsersByIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]Profile, error)
}
