package adapters

import (
	"context"
	"fmt"
	"strings"

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

	consumerEmail, consumerName, consumerPhone := extractConsumerContact(lead)

	org, err := a.orgs.GetOrganization(ctx, organizationID)
	if err != nil {
		return quotesvc.QuoteContactData{}, fmt.Errorf("look up organization for quote email: %w", err)
	}

	agentEmail, agentName := a.lookupAgentContact(ctx, lead.AssignedAgentID)

	return quotesvc.QuoteContactData{
		ConsumerEmail:    consumerEmail,
		ConsumerName:     consumerName,
		ConsumerPhone:    consumerPhone,
		OrganizationName: org.Name,
		AgentEmail:       agentEmail,
		AgentName:        agentName,
	}, nil
}

func extractConsumerContact(lead leadsrepo.Lead) (string, string, string) {
	consumerEmail := ""
	if lead.ConsumerEmail != nil {
		consumerEmail = *lead.ConsumerEmail
	}

	consumerName := strings.TrimSpace(lead.ConsumerFirstName + " " + lead.ConsumerLastName)
	if consumerName == "" {
		consumerName = lead.ConsumerFirstName
	}

	return consumerEmail, consumerName, lead.ConsumerPhone
}

func (a *QuotesContactReader) lookupAgentContact(ctx context.Context, agentID *uuid.UUID) (string, string) {
	if agentID == nil || a.users == nil {
		return "", ""
	}

	user, err := a.users.GetUserByID(ctx, *agentID)
	if err != nil {
		return "", ""
	}

	name := buildUserName(user)
	if name == "" {
		name = user.Email
	}

	return user.Email, name
}

func buildUserName(user authrepo.User) string {
	first := ""
	if user.FirstName != nil {
		first = *user.FirstName
	}

	last := ""
	if user.LastName != nil {
		last = *user.LastName
	}

	return strings.TrimSpace(first + " " + last)
}

// Compile-time check that QuotesContactReader implements quotes/service.QuoteContactReader.
var _ quotesvc.QuoteContactReader = (*QuotesContactReader)(nil)
