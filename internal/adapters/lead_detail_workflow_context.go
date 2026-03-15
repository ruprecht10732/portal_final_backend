package adapters

import (
	"context"

	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

type leadDetailWorkflowContextService interface {
	GetLeadWorkflowOverride(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (identityrepo.LeadWorkflowOverride, error)
	ResolveLeadWorkflow(ctx context.Context, input identityservice.ResolveLeadWorkflowInput) (identityservice.ResolveLeadWorkflowResult, error)
}

type LeadDetailWorkflowContextReader struct {
	svc leadDetailWorkflowContextService
}

func NewLeadDetailWorkflowContextReader(svc leadDetailWorkflowContextService) *LeadDetailWorkflowContextReader {
	return &LeadDetailWorkflowContextReader{svc: svc}
}

func (r *LeadDetailWorkflowContextReader) GetLeadWorkflowContext(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID, leadSource *string, leadServiceType *string, pipelineStage *string) (*transport.LeadDetailWorkflowContext, error) {
	if r == nil || r.svc == nil {
		return nil, nil
	}

	resolved, err := r.svc.ResolveLeadWorkflow(ctx, identityservice.ResolveLeadWorkflowInput{
		OrganizationID:  tenantID,
		LeadID:          leadID,
		LeadSource:      leadSource,
		LeadServiceType: leadServiceType,
		PipelineStage:   pipelineStage,
	})
	if err != nil {
		return nil, err
	}

	contextResponse := &transport.LeadDetailWorkflowContext{
		Resolved: mapLeadWorkflowResolutionContext(resolved),
	}

	override, err := r.svc.GetLeadWorkflowOverride(ctx, leadID, tenantID)
	if err == nil {
		contextResponse.Override = mapLeadWorkflowOverrideContext(override)
		return contextResponse, nil
	}
	if apperr.Is(err, apperr.KindNotFound) {
		return contextResponse, nil
	}
	return nil, err
}

func mapLeadWorkflowOverrideContext(override identityrepo.LeadWorkflowOverride) *transport.LeadDetailWorkflowOverrideContext {
	return &transport.LeadDetailWorkflowOverrideContext{
		WorkflowID:   workflowUUIDPtrToString(override.WorkflowID),
		OverrideMode: override.OverrideMode,
	}
}

func mapLeadWorkflowResolutionContext(result identityservice.ResolveLeadWorkflowResult) *transport.LeadDetailWorkflowResolutionContext {
	contextResponse := &transport.LeadDetailWorkflowResolutionContext{
		ResolutionSource: result.ResolutionSource,
		OverrideMode:     result.OverrideMode,
		MatchedRuleID:    workflowUUIDPtrToString(result.MatchedRuleID),
	}
	if result.Workflow != nil {
		workflowID := result.Workflow.ID.String()
		workflowName := result.Workflow.Name
		contextResponse.WorkflowID = &workflowID
		contextResponse.WorkflowName = &workflowName
	}
	return contextResponse
}

func workflowUUIDPtrToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	value := id.String()
	return &value
}
