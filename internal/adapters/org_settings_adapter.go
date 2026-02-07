package adapters

import (
	"context"

	identityrepo "portal_final_backend/internal/identity/repository"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// OrgSettingsReaderRepo is the narrow interface for fetching organization settings.
type OrgSettingsReaderRepo interface {
	GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (identityrepo.OrganizationSettings, error)
}

// OrgSettingsAdapter implements quotes/service.OrgSettingsReader using the
// identity service's organization settings.
type OrgSettingsAdapter struct {
	svc OrgSettingsReaderRepo
}

// NewOrgSettingsAdapter creates a new adapter.
func NewOrgSettingsAdapter(svc OrgSettingsReaderRepo) *OrgSettingsAdapter {
	return &OrgSettingsAdapter{svc: svc}
}

// GetQuoteDefaults returns the organization's configurable quote defaults.
func (a *OrgSettingsAdapter) GetQuoteDefaults(ctx context.Context, organizationID uuid.UUID) (paymentDays int, validDays int, err error) {
	s, err := a.svc.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return 7, 14, err
	}
	return s.QuotePaymentDays, s.QuoteValidDays, nil
}

// Compile-time check.
var _ quotesvc.OrgSettingsReader = (*OrgSettingsAdapter)(nil)
