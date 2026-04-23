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
type UserProvider interface {
	GetUserByID(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}

// Agent represents the agent information that the RAC_leads domain needs.
type Agent struct {
	ID    uuid.UUID
	Email string
	Name  string
}

// AgentProvider is the interface that the RAC_leads domain uses to get agent information.
type AgentProvider interface {
	GetAgentByID(ctx context.Context, agentID uuid.UUID) (Agent, error)
	GetAgentsByIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID]Agent, error)
	ListAgents(ctx context.Context) ([]Agent, error)
}
