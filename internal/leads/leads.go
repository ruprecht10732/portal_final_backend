// Package leads provides lead management functionality.
// This file defines the public API of the leads bounded context.
// Only types and interfaces defined here should be imported by other domains.
package leads

import (
	"context"

	"github.com/google/uuid"
)

// Lead represents the minimal lead information that can be shared with other domains.
type Lead struct {
	ID              uuid.UUID
	ConsumerName    string
	AssignedAgentID *uuid.UUID
}

// Service defines the public interface for lead operations.
// Other domains should depend on this interface, not on concrete implementations.
type Service interface {
	// GetLeadByID returns minimal lead information for a given ID.
	GetLeadByID(ctx context.Context, id uuid.UUID) (Lead, error)
	// GetLeadsForAgent returns leads assigned to a specific agent.
	GetLeadsForAgent(ctx context.Context, agentID uuid.UUID) ([]Lead, error)
}

// Note: The full leads service with all CRUD operations is intended for use
// within the HTTP handler layer only. Other domains should use the minimal
// Service interface above or define their own interfaces for the specific
// data they need (see AgentProvider pattern below).
