package handler

import (
	"net/http"

	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const msgInvalidLeadID = "invalid lead id"

func (h *Handler) ListWorkflows(c *gin.Context) {
	tenantID, ok := h.requireTenantID(c)
	if !ok {
		return
	}

	workflows, err := h.svc.ListWorkflows(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	resp := make([]transport.WorkflowResponse, 0, len(workflows))
	for _, workflow := range workflows {
		resp = append(resp, mapWorkflowResponse(workflow))
	}

	httpkit.OK(c, transport.ListWorkflowsResponse{Workflows: resp})
}

func (h *Handler) ReplaceWorkflows(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	if !h.canManageWorkflowEngine(c, *tenantID, identity.UserID(), identity.HasRole("admin")) {
		return
	}

	var req transport.ReplaceWorkflowsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	upserts := make([]repository.WorkflowUpsert, 0, len(req.Workflows))
	for _, workflow := range req.Workflows {
		workflowUpsert, err := mapWorkflowUpsertRequest(workflow)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
			return
		}
		upserts = append(upserts, workflowUpsert)
	}

	workflows, err := h.svc.ReplaceWorkflows(c.Request.Context(), *tenantID, upserts)
	if httpkit.HandleError(c, err) {
		return
	}

	resp := make([]transport.WorkflowResponse, 0, len(workflows))
	for _, workflow := range workflows {
		resp = append(resp, mapWorkflowResponse(workflow))
	}

	httpkit.OK(c, transport.ListWorkflowsResponse{Workflows: resp})
}

func (h *Handler) ListWorkflowAssignmentRules(c *gin.Context) {
	tenantID, ok := h.requireTenantID(c)
	if !ok {
		return
	}

	rules, err := h.svc.ListWorkflowAssignmentRules(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	resp := make([]transport.WorkflowAssignmentRuleResponse, 0, len(rules))
	for _, rule := range rules {
		resp = append(resp, mapWorkflowAssignmentRuleResponse(rule))
	}

	httpkit.OK(c, transport.ListWorkflowAssignmentRulesResponse{Rules: resp})
}

func (h *Handler) ReplaceWorkflowAssignmentRules(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	if !h.canManageWorkflowEngine(c, *tenantID, identity.UserID(), identity.HasRole("admin")) {
		return
	}

	var req transport.ReplaceWorkflowAssignmentRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	upserts := make([]repository.WorkflowAssignmentRuleUpsert, 0, len(req.Rules))
	for _, rule := range req.Rules {
		workflowID, err := uuid.Parse(rule.WorkflowID)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "invalid workflowId")
			return
		}

		upsert := repository.WorkflowAssignmentRuleUpsert{
			Name:            rule.Name,
			Enabled:         rule.Enabled,
			Priority:        rule.Priority,
			WorkflowID:      workflowID,
			LeadSource:      rule.LeadSource,
			LeadServiceType: rule.LeadServiceType,
			PipelineStage:   rule.PipelineStage,
		}

		if rule.ID != nil {
			parsedRuleID, err := uuid.Parse(*rule.ID)
			if err != nil {
				httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "invalid rule id")
				return
			}
			upsert.ID = &parsedRuleID
		}

		upserts = append(upserts, upsert)
	}

	rules, err := h.svc.ReplaceWorkflowAssignmentRules(c.Request.Context(), *tenantID, upserts)
	if httpkit.HandleError(c, err) {
		return
	}

	resp := make([]transport.WorkflowAssignmentRuleResponse, 0, len(rules))
	for _, rule := range rules {
		resp = append(resp, mapWorkflowAssignmentRuleResponse(rule))
	}

	httpkit.OK(c, transport.ListWorkflowAssignmentRulesResponse{Rules: resp})
}

func (h *Handler) GetLeadWorkflowOverride(c *gin.Context) {
	tenantID, ok := h.requireTenantID(c)
	if !ok {
		return
	}

	leadID, err := uuid.Parse(c.Param("leadID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, msgInvalidLeadID)
		return
	}

	override, err := h.svc.GetLeadWorkflowOverride(c.Request.Context(), leadID, tenantID)
	if err != nil {
		// Missing overrides are normal (most leads won't have one). Return null instead
		// of a 404 to keep the endpoint easy to consume.
		if apperr.Is(err, apperr.KindNotFound) {
			httpkit.OK(c, nil)
			return
		}
		if httpkit.HandleError(c, err) {
			return
		}
	}

	httpkit.OK(c, mapLeadWorkflowOverrideResponse(override))
}

func (h *Handler) UpsertLeadWorkflowOverride(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	if !h.canManageWorkflowEngine(c, *tenantID, identity.UserID(), identity.HasRole("admin")) {
		return
	}

	leadID, err := uuid.Parse(c.Param("leadID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, msgInvalidLeadID)
		return
	}

	var req transport.UpsertLeadWorkflowOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if req.LeadID != "" && req.LeadID != leadID.String() {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "leadId must match path")
		return
	}
	req.LeadID = leadID.String()
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	var workflowID *uuid.UUID
	if req.WorkflowID != nil {
		parsedWorkflowID, err := uuid.Parse(*req.WorkflowID)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, "invalid workflowId")
			return
		}
		workflowID = &parsedWorkflowID
	}

	assignedBy := identity.UserID()
	override, err := h.svc.UpsertLeadWorkflowOverride(c.Request.Context(), repository.LeadWorkflowOverrideUpsert{
		LeadID:         leadID,
		OrganizationID: *tenantID,
		WorkflowID:     workflowID,
		OverrideMode:   req.OverrideMode,
		Reason:         req.Reason,
		AssignedBy:     &assignedBy,
	})
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, mapLeadWorkflowOverrideResponse(override))
}

func (h *Handler) DeleteLeadWorkflowOverride(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return
	}

	if !h.canManageWorkflowEngine(c, *tenantID, identity.UserID(), identity.HasRole("admin")) {
		return
	}

	leadID, err := uuid.Parse(c.Param("leadID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, msgInvalidLeadID)
		return
	}

	if err := h.svc.DeleteLeadWorkflowOverride(c.Request.Context(), leadID, *tenantID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "deleted"})
}

func (h *Handler) ResolveLeadWorkflow(c *gin.Context) {
	tenantID, ok := h.requireTenantID(c)
	if !ok {
		return
	}

	leadID, err := uuid.Parse(c.Param("leadID"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, msgInvalidLeadID)
		return
	}

	leadSource := optionalQuery(c, "leadSource")
	leadServiceType := optionalQuery(c, "leadServiceType")
	pipelineStage := optionalQuery(c, "pipelineStage")

	resolved, err := h.svc.ResolveLeadWorkflow(c.Request.Context(), service.ResolveLeadWorkflowInput{
		OrganizationID:  tenantID,
		LeadID:          leadID,
		LeadSource:      leadSource,
		LeadServiceType: leadServiceType,
		PipelineStage:   pipelineStage,
	})
	if httpkit.HandleError(c, err) {
		return
	}

	resp := transport.ResolveLeadWorkflowResponse{
		ResolutionSource: resolved.ResolutionSource,
		OverrideMode:     resolved.OverrideMode,
	}
	if resolved.Workflow != nil {
		workflow := mapWorkflowResponse(*resolved.Workflow)
		resp.Workflow = &workflow
	}
	if resolved.MatchedRuleID != nil {
		id := resolved.MatchedRuleID.String()
		resp.MatchedRuleID = &id
	}

	httpkit.OK(c, resp)
}

func (h *Handler) requireTenantID(c *gin.Context) (uuid.UUID, bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return uuid.Nil, false
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, msgTenantNotSet, nil)
		return uuid.Nil, false
	}
	return *tenantID, true
}

func (h *Handler) canManageWorkflowEngine(c *gin.Context, tenantID, userID uuid.UUID, hasAdminRole bool) bool {
	org, err := h.svc.GetOrganization(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return false
	}
	if org.CreatedBy != userID && !hasAdminRole {
		httpkit.Error(c, http.StatusForbidden, "forbidden", nil)
		return false
	}
	return true
}

func mapWorkflowResponse(workflow repository.Workflow) transport.WorkflowResponse {
	steps := make([]transport.WorkflowStepResponse, 0, len(workflow.Steps))
	for _, step := range workflow.Steps {
		steps = append(steps, mapWorkflowStepResponse(step))
	}

	return transport.WorkflowResponse{
		ID:                       workflow.ID.String(),
		WorkflowKey:              workflow.WorkflowKey,
		Name:                     workflow.Name,
		Description:              workflow.Description,
		Enabled:                  workflow.Enabled,
		QuoteValidDaysOverride:   workflow.QuoteValidDaysOverride,
		QuotePaymentDaysOverride: workflow.QuotePaymentDaysOverride,
		Steps:                    steps,
		CreatedAt:                workflow.CreatedAt,
		UpdatedAt:                workflow.UpdatedAt,
	}
}

func mapWorkflowStepResponse(step repository.WorkflowStep) transport.WorkflowStepResponse {
	cfg := transport.WorkflowStepRecipientConfig{}
	if audience, ok := step.RecipientConfig["audience"].(string); ok {
		cfg.Audience = audience
	}
	if includeAssignedAgent, ok := step.RecipientConfig["includeAssignedAgent"].(bool); ok {
		cfg.IncludeAssignedAgent = includeAssignedAgent
	}
	if includeLeadContact, ok := step.RecipientConfig["includeLeadContact"].(bool); ok {
		cfg.IncludeLeadContact = includeLeadContact
	}
	if includePartner, ok := step.RecipientConfig["includePartner"].(bool); ok {
		cfg.IncludePartner = includePartner
	}
	if includeInternal, ok := step.RecipientConfig["includeInternal"].(bool); ok {
		cfg.IncludeInternal = includeInternal
	}
	cfg.CustomEmails = stringSliceFromAny(step.RecipientConfig["customEmails"])
	cfg.CustomPhones = stringSliceFromAny(step.RecipientConfig["customPhones"])

	return transport.WorkflowStepResponse{
		ID:              step.ID.String(),
		Trigger:         step.Trigger,
		Channel:         step.Channel,
		Audience:        step.Audience,
		Action:          step.Action,
		StepOrder:       step.StepOrder,
		DelayMinutes:    step.DelayMinutes,
		Enabled:         step.Enabled,
		RecipientConfig: cfg,
		TemplateSubject: step.TemplateSubject,
		TemplateBody:    step.TemplateBody,
		StopOnReply:     step.StopOnReply,
	}
}

func mapWorkflowUpsertRequest(req transport.UpsertWorkflowRequest) (repository.WorkflowUpsert, error) {
	upsert := repository.WorkflowUpsert{
		WorkflowKey:              req.WorkflowKey,
		Name:                     req.Name,
		Description:              req.Description,
		Enabled:                  req.Enabled,
		QuoteValidDaysOverride:   req.QuoteValidDaysOverride,
		QuotePaymentDaysOverride: req.QuotePaymentDaysOverride,
		Steps:                    make([]repository.WorkflowStepUpsert, 0, len(req.Steps)),
	}

	if req.ID != nil {
		id, err := uuid.Parse(*req.ID)
		if err != nil {
			return repository.WorkflowUpsert{}, err
		}
		upsert.ID = &id
	}

	for _, stepReq := range req.Steps {
		stepUpsert, err := mapWorkflowStepUpsertRequest(stepReq)
		if err != nil {
			return repository.WorkflowUpsert{}, err
		}
		upsert.Steps = append(upsert.Steps, stepUpsert)
	}

	return upsert, nil
}

func mapWorkflowStepUpsertRequest(req transport.UpsertWorkflowStepRequest) (repository.WorkflowStepUpsert, error) {
	step := repository.WorkflowStepUpsert{
		Trigger:         req.Trigger,
		Channel:         req.Channel,
		Audience:        req.Audience,
		Action:          req.Action,
		StepOrder:       req.StepOrder,
		DelayMinutes:    req.DelayMinutes,
		Enabled:         req.Enabled,
		TemplateSubject: req.TemplateSubject,
		TemplateBody:    req.TemplateBody,
		StopOnReply:     req.StopOnReply,
		RecipientConfig: map[string]any{},
	}

	if req.ID != nil {
		id, err := uuid.Parse(*req.ID)
		if err != nil {
			return repository.WorkflowStepUpsert{}, err
		}
		step.ID = &id
	}

	if req.RecipientConfig.Audience != "" {
		step.RecipientConfig["audience"] = req.RecipientConfig.Audience
	}
	step.RecipientConfig["includeAssignedAgent"] = req.RecipientConfig.IncludeAssignedAgent
	step.RecipientConfig["includeLeadContact"] = req.RecipientConfig.IncludeLeadContact
	step.RecipientConfig["includePartner"] = req.RecipientConfig.IncludePartner
	step.RecipientConfig["includeInternal"] = req.RecipientConfig.IncludeInternal
	if len(req.RecipientConfig.CustomEmails) > 0 {
		step.RecipientConfig["customEmails"] = req.RecipientConfig.CustomEmails
	}
	if len(req.RecipientConfig.CustomPhones) > 0 {
		step.RecipientConfig["customPhones"] = req.RecipientConfig.CustomPhones
	}

	return step, nil
}

func mapWorkflowAssignmentRuleResponse(rule repository.WorkflowAssignmentRule) transport.WorkflowAssignmentRuleResponse {
	return transport.WorkflowAssignmentRuleResponse{
		ID:              rule.ID.String(),
		WorkflowID:      rule.WorkflowID.String(),
		Name:            rule.Name,
		Enabled:         rule.Enabled,
		Priority:        rule.Priority,
		LeadSource:      rule.LeadSource,
		LeadServiceType: rule.LeadServiceType,
		PipelineStage:   rule.PipelineStage,
		CreatedAt:       rule.CreatedAt,
		UpdatedAt:       rule.UpdatedAt,
	}
}

func mapLeadWorkflowOverrideResponse(override repository.LeadWorkflowOverride) transport.LeadWorkflowOverrideResponse {
	var workflowID *string
	if override.WorkflowID != nil {
		id := override.WorkflowID.String()
		workflowID = &id
	}

	var assignedBy *string
	if override.AssignedBy != nil {
		id := override.AssignedBy.String()
		assignedBy = &id
	}

	return transport.LeadWorkflowOverrideResponse{
		LeadID:       override.LeadID.String(),
		WorkflowID:   workflowID,
		OverrideMode: override.OverrideMode,
		Reason:       override.Reason,
		AssignedBy:   assignedBy,
		CreatedAt:    override.CreatedAt,
		UpdatedAt:    override.UpdatedAt,
	}
}

func optionalQuery(c *gin.Context, key string) *string {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	return &value
}

func stringSliceFromAny(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		result = append(result, text)
	}
	return result
}
