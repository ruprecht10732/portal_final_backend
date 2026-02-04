// Package ports defines the interfaces that the RAC_leads domain requires from
// external systems. These interfaces form the Anti-Corruption Layer (ACL),
// ensuring the RAC_leads domain only knows about the data it needs, formatted
// the way it wants.
package ports

import (
	"context"

	"github.com/google/uuid"
)

// Agent represents the agent information that the RAC_leads domain needs.
// This is defined by the RAC_leads domain, not by the auth domain.
type Agent struct {
	ID    uuid.UUID
	Email string
	Name  string // Display name, can be derived from email if needed
}

// AgentProvider is the interface that the RAC_leads domain uses to get agent information.
// The implementation is provided by the composition root (main/router) and wraps
// the auth service. This ensures RAC_leads never directly imports the auth domain.
type AgentProvider interface {
	// GetAgentByID returns agent information for the given user ID.
	// Returns an error if the agent is not found.
	GetAgentByID(ctx context.Context, agentID uuid.UUID) (Agent, error)

	// GetAgentsByIDs returns agent information for multiple user IDs.
	// Missing agents are silently omitted from the result map.
	GetAgentsByIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID]Agent, error)

	// ListAgents returns all available agents that can be assigned to RAC_leads.
	ListAgents(ctx context.Context) ([]Agent, error)
}
