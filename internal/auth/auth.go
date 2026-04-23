// Package auth provides authentication and authorization functionality.
// This file defines the public API (Bounded Context) of the auth domain.
// Only types and interfaces defined here should be imported by other domains.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Profile represents user information shareable across domains.
// Struct fields are sorted by size (descending) to optimize memory alignment.
// This eliminates padding bytes, reducing memory footprint per instance (O(1) space optimization),
// which significantly lowers Garbage Collection (GC) pressure in high-throughput systems.
type Profile struct {
	CreatedAt     time.Time // 24 bytes
	UpdatedAt     time.Time // 24 bytes
	Roles         []string  // 24 bytes
	ID            uuid.UUID // 16 bytes
	Email         string    // 16 bytes
	PreferredLang string    // 16 bytes
	FirstName     *string   // 8 bytes
	LastName      *string   // 8 bytes
	Phone         *string   // 8 bytes
	EmailVerified bool      // 1 byte
}

// UserSummary represents minimal user information for listing purposes.
type UserSummary struct {
	Roles []string `json:"roles"` // 24 bytes
	ID    string   `json:"id"`    // 16 bytes (Tech Debt: Consider migrating to uuid.UUID for type safety)
	Email string   `json:"email"` // 16 bytes
}

// ListOptions enforces pagination to prevent memory exhaustion and database lockups.
type ListOptions struct {
	Limit  uint // uint prevents negative limits
	Offset uint
}

// Service defines the public API for authentication operations.
// Intended for primary adapters (e.g., HTTP/gRPC handlers) within this bounded context.
type Service interface {
	// GetMe returns the profile of the user with the given ID.
	GetMe(ctx context.Context, userID uuid.UUID) (Profile, error)

	// Deprecated: ListUsers is strictly O(N) where N is the total number of users.
	// As the system scales, this becomes a severe DoS vector and memory leak.
	// Retained for backwards compatibility. Use ListUsersPaginated instead.
	ListUsers(ctx context.Context) ([]UserSummary, error)

	// ListUsersPaginated returns a bounded list of users (O(limit) space/time complexity).
	ListUsersPaginated(ctx context.Context, opts ListOptions) ([]UserSummary, error)
}

// UserProvider is the Anti-Corruption Layer (ACL) for cross-domain communication.
// Other bounded contexts depend on this to abstract away authentication details.
type UserProvider interface {
	// GetUserByID returns basic user information needed by other domains.
	GetUserByID(ctx context.Context, userID uuid.UUID) (Profile, error)

	// GetUsersByIDs returns user information for multiple users at once.
	// Implementations MUST batch the underlying DB query (e.g., WHERE id IN (...))
	// to ensure O(1) network roundtrips rather than an O(N) N+1 query loop.
	GetUsersByIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]Profile, error)
}
