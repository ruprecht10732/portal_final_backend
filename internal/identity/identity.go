// Package identity provides the identity and tenancy bounded context API.
package identity

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the public interface for tenancy operations.
// Other domains should depend on this interface, not on concrete implementations.
type Service interface {
	// GetUserOrganizationID returns the organization ID for a user.
	GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
