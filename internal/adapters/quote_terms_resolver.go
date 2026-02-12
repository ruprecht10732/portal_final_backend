package adapters

import (
	"context"

	identityrepo "portal_final_backend/internal/identity/repository"
	identitysvc "portal_final_backend/internal/identity/service"
	leadsrepo "portal_final_backend/internal/leads/repository"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuoteTermsSettingsReader reads organization default quote terms.
type QuoteTermsSettingsReader interface {
	GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (identityrepo.OrganizationSettings, error)
}

// QuoteTermsWorkflowResolver resolves effective workflow for a lead.
type QuoteTermsWorkflowResolver interface {
	ResolveLeadWorkflow(ctx context.Context, input identitysvc.ResolveLeadWorkflowInput) (identitysvc.ResolveLeadWorkflowResult, error)
}

// QuoteLeadServiceReader reads lead-service context used for assignment matching.
type QuoteLeadServiceReader interface {
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (leadsrepo.LeadService, error)
	GetCurrentLeadService(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (leadsrepo.LeadService, error)
}

// QuoteTermsResolverAdapter resolves effective quote terms with precedence:
// workflow override > organization defaults.
type QuoteTermsResolverAdapter struct {
	settingsReader   QuoteTermsSettingsReader
	workflowResolver QuoteTermsWorkflowResolver
	leadServices     QuoteLeadServiceReader
}

func NewQuoteTermsResolverAdapter(
	settingsReader QuoteTermsSettingsReader,
	workflowResolver QuoteTermsWorkflowResolver,
	leadServices QuoteLeadServiceReader,
) *QuoteTermsResolverAdapter {
	return &QuoteTermsResolverAdapter{
		settingsReader:   settingsReader,
		workflowResolver: workflowResolver,
		leadServices:     leadServices,
	}
}

func (a *QuoteTermsResolverAdapter) ResolveQuoteTerms(
	ctx context.Context,
	organizationID uuid.UUID,
	leadID uuid.UUID,
	leadServiceID *uuid.UUID,
) (paymentDays int, validDays int, err error) {
	settings, err := a.settingsReader.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return 7, 14, err
	}

	paymentDays = settings.QuotePaymentDays
	validDays = settings.QuoteValidDays

	if a.workflowResolver == nil || a.leadServices == nil {
		return paymentDays, validDays, nil
	}

	serviceContext, ok := a.resolveLeadServiceContext(ctx, leadID, organizationID, leadServiceID)
	if !ok {
		return paymentDays, validDays, nil
	}

	resolved, err := a.workflowResolver.ResolveLeadWorkflow(ctx, identitysvc.ResolveLeadWorkflowInput{
		OrganizationID:  organizationID,
		LeadID:          leadID,
		LeadSource:      serviceContext.Source,
		LeadServiceType: &serviceContext.ServiceType,
		PipelineStage:   &serviceContext.PipelineStage,
	})
	if err != nil || resolved.Workflow == nil {
		return paymentDays, validDays, nil
	}

	if resolved.Workflow.QuotePaymentDaysOverride != nil {
		paymentDays = *resolved.Workflow.QuotePaymentDaysOverride
	}
	if resolved.Workflow.QuoteValidDaysOverride != nil {
		validDays = *resolved.Workflow.QuoteValidDaysOverride
	}

	return paymentDays, validDays, nil
}

func (a *QuoteTermsResolverAdapter) resolveLeadServiceContext(
	ctx context.Context,
	leadID uuid.UUID,
	organizationID uuid.UUID,
	leadServiceID *uuid.UUID,
) (leadsrepo.LeadService, bool) {
	if leadServiceID != nil {
		svc, err := a.leadServices.GetLeadServiceByID(ctx, *leadServiceID, organizationID)
		if err == nil {
			return svc, true
		}
	}

	svc, err := a.leadServices.GetCurrentLeadService(ctx, leadID, organizationID)
	if err != nil {
		return leadsrepo.LeadService{}, false
	}
	return svc, true
}

var _ quotesvc.QuoteTermsResolver = (*QuoteTermsResolverAdapter)(nil)
