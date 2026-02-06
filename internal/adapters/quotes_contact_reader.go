package adapters

import (
	"context"
	"fmt"

	authrepo "portal_final_backend/internal/auth/repository"
	identityrepo "portal_final_backend/internal/identity/repository"
	leadsrepo "portal_final_backend/internal/leads/repository"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuotesContactReader adapts the leads, identity, and auth repositories
// to provide contact details needed for quote emails (consumer + organization + agent).
// It implements quotes/service.QuoteContactReader using interface-segregation.
type QuotesContactReader struct {
	leads leadsrepo.LeadReader
	orgs  OrgNameReader
	users AgentUserReader
}

// OrgNameReader is the narrow interface for fetching an organization name.
type OrgNameReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
}

// AgentUserReader is the narrow interface for looking up an agent's email and name by ID.
type AgentUserReader interface {
	GetUserByID(ctx context.Context, userID uuid.UUID) (authrepo.User, error)
}

// NewQuotesContactReader creates a new contact reader adapter.
func NewQuotesContactReader(leads leadsrepo.LeadReader, orgs OrgNameReader, users AgentUserReader) *QuotesContactReader {
	return &QuotesContactReader{leads: leads, orgs: orgs, users: users}
}

// GetQuoteContactData retrieves consumer email, consumer name, organization name,
// and the assigned agent's email and name.
func (a *QuotesContactReader) GetQuoteContactData(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (quotesvc.QuoteContactData, error) {
	lead, err := a.leads.GetByID(ctx, leadID, organizationID)
	if err != nil {
		return quotesvc.QuoteContactData{}, fmt.Errorf("look up lead for quote email: %w", err)
	}

	consumerEmail := ""
	if lead.ConsumerEmail != nil {
		consumerEmail = *lead.ConsumerEmail
	}
	consumerName := lead.ConsumerFirstName
	if lead.ConsumerLastName != "" {
		consumerName += " " + lead.ConsumerLastName
	}

	org, err := a.orgs.GetOrganization(ctx, organizationID)
	if err != nil {
		return quotesvc.QuoteContactData{}, fmt.Errorf("look up organization for quote email: %w", err)
	}

	// Look up the assigned agent's email and name
	var agentEmail, agentName string
	if lead.AssignedAgentID != nil && a.users != nil {
		user, userErr := a.users.GetUserByID(ctx, *lead.AssignedAgentID)
		if userErr == nil {
			agentEmail = user.Email
			if user.FirstName != nil {
				agentName = *user.FirstName
			}
			if user.LastName != nil {
				if agentName != "" {
					agentName += " "
				}
				agentName += *user.LastName
			}
			if agentName == "" {
				agentName = user.Email
			}
		}
	}

	return quotesvc.QuoteContactData{
		ConsumerEmail:    consumerEmail,
		ConsumerName:     consumerName,
		OrganizationName: org.Name,
		AgentEmail:       agentEmail,
		AgentName:        agentName,
	}, nil
}

// Compile-time check that QuotesContactReader implements quotes/service.QuoteContactReader.
var _ quotesvc.QuoteContactReader = (*QuotesContactReader)(nil)
