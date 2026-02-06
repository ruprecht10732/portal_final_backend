package adapters

import (
	"context"
	"fmt"

	identityrepo "portal_final_backend/internal/identity/repository"
	leadsrepo "portal_final_backend/internal/leads/repository"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuotesContactReader adapts the leads and identity repositories
// to provide contact details needed for quote proposal emails.
// It implements quotes/service.QuoteContactReader using interface-segregation.
type QuotesContactReader struct {
	leads leadsrepo.LeadReader
	orgs  OrgNameReader
}

// OrgNameReader is the narrow interface for fetching an organization name.
type OrgNameReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
}

// NewQuotesContactReader creates a new contact reader adapter.
func NewQuotesContactReader(leads leadsrepo.LeadReader, orgs OrgNameReader) *QuotesContactReader {
	return &QuotesContactReader{leads: leads, orgs: orgs}
}

// GetQuoteContactData retrieves consumer email, consumer name, and organization name.
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

	return quotesvc.QuoteContactData{
		ConsumerEmail:    consumerEmail,
		ConsumerName:     consumerName,
		OrganizationName: org.Name,
	}, nil
}

// Compile-time check that QuotesContactReader implements quotes/service.QuoteContactReader.
var _ quotesvc.QuoteContactReader = (*QuotesContactReader)(nil)
