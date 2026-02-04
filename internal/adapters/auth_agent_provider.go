// Package adapters contains adapters that bridge different bounded contexts.
// These adapters implement interfaces defined by consuming domains while
// wrapping services from providing domains.
package adapters

import (
	"context"
	"strings"

	authservice "portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// AuthAgentProvider adapts the auth service to satisfy the RAC_leads domain's
// AgentProvider interface. This is the Anti-Corruption Layer implementation
// that ensures the RAC_leads domain doesn't need to know about auth domain internals.
type AuthAgentProvider struct {
	authSvc *authservice.Service
}

// NewAuthAgentProvider creates a new adapter wrapping the auth service.
func NewAuthAgentProvider(authSvc *authservice.Service) *AuthAgentProvider {
	return &AuthAgentProvider{authSvc: authSvc}
}

// GetAgentByID returns agent information for the given user ID.
func (p *AuthAgentProvider) GetAgentByID(ctx context.Context, agentID uuid.UUID) (ports.Agent, error) {
	profile, err := p.authSvc.GetMe(ctx, agentID)
	if err != nil {
		return ports.Agent{}, err
	}

	return ports.Agent{
		ID:    profile.ID,
		Email: profile.Email,
		Name:  buildDisplayName(profile.FirstName, profile.LastName, profile.Email),
	}, nil
}

// GetAgentsByIDs returns agent information for multiple user IDs.
func (p *AuthAgentProvider) GetAgentsByIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID]ports.Agent, error) {
	result := make(map[uuid.UUID]ports.Agent)

	for _, id := range agentIDs {
		agent, err := p.GetAgentByID(ctx, id)
		if err != nil {
			// Silently omit missing agents
			continue
		}
		result[id] = agent
	}

	return result, nil
}

// ListAgents returns all available agents.
func (p *AuthAgentProvider) ListAgents(ctx context.Context) ([]ports.Agent, error) {
	RAC_users, err := p.authSvc.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	agents := make([]ports.Agent, 0, len(RAC_users))
	for _, user := range RAC_users {
		id, err := uuid.Parse(user.ID)
		if err != nil {
			continue
		}
		agents = append(agents, ports.Agent{
			ID:    id,
			Email: user.Email,
			Name:  buildDisplayName(user.FirstName, user.LastName, user.Email),
		})
	}

	return agents, nil
}

// deriveNameFromEmail creates a display name from an email address.
func buildDisplayName(firstName, lastName *string, email string) string {
	first := ""
	last := ""
	if firstName != nil {
		first = strings.TrimSpace(*firstName)
	}
	if lastName != nil {
		last = strings.TrimSpace(*lastName)
	}
	full := strings.TrimSpace(strings.Join([]string{first, last}, " "))
	if full != "" {
		return full
	}
	return deriveNameFromEmail(email)
}

func deriveNameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return email
	}
	name := parts[0]
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return strings.Title(name)
}

// Compile-time check that AuthAgentProvider implements ports.AgentProvider
var _ ports.AgentProvider = (*AuthAgentProvider)(nil)
