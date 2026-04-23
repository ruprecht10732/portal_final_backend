package notification

import (
	"context"
	"fmt"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WorkflowResolver interface {
	ResolveLeadWorkflow(ctx context.Context, input identityservice.ResolveLeadWorkflowInput) (identityservice.ResolveLeadWorkflowResult, error)
}

type workflowRule struct {
	Enabled         bool
	DelayMinutes    int
	TemplateSubject *string
	TemplateText    *string
}

type workflowStepExecutionContext struct {
	OrgID          uuid.UUID
	LeadID         *uuid.UUID
	ServiceID      *uuid.UUID
	LeadPhone      string
	LeadEmail      string
	PartnerPhone   string
	PartnerEmail   string
	Trigger        string
	DefaultSummary string
	DefaultActor   string
	DefaultOrigin  string
	Variables      map[string]any
}

type workflowStepDispatchContext struct {
	Step      repository.WorkflowStep
	Exec      workflowStepExecutionContext
	RunAt     time.Time
	Body      string
	Category  string
	Audience  string
	Summary   string
	ActorType string
	ActorName string
}

func (m *Module) resolveWorkflowRule(
	ctx context.Context,
	orgID uuid.UUID,
	leadID uuid.UUID,
	trigger string,
	channel string,
	audience string,
	leadSource *string,
) *workflowRule {
	if m.workflowResolver == nil {
		m.log.Debug("workflow resolver not configured", "orgId", orgID, "leadId", leadID, "trigger", trigger, "channel", channel, "audience", audience)
		return nil
	}

	resolved, err := m.workflowResolver.ResolveLeadWorkflow(ctx, identityservice.ResolveLeadWorkflowInput{
		OrganizationID: orgID,
		LeadID:         leadID,
		LeadSource:     leadSource,
	})
	if err != nil {
		m.log.Warn("failed to resolve lead workflow", "error", err, "orgId", orgID, "leadId", leadID, "trigger", trigger)
		return nil
	}

	if resolved.Workflow == nil {
		m.log.Debug("no workflow resolved for lead", "orgId", orgID, "leadId", leadID, "trigger", trigger, "resolutionSource", resolved.ResolutionSource)
		return nil
	}

	for _, step := range resolved.Workflow.Steps {
		if step.Trigger != trigger {
			continue
		}
		if !strings.EqualFold(step.Channel, channel) || !strings.EqualFold(step.Audience, audience) {
			continue
		}

		subjectLen := 0
		subjectTrimLen := 0
		if step.TemplateSubject != nil {
			subjectLen = len(*step.TemplateSubject)
			subjectTrimLen = len(strings.TrimSpace(*step.TemplateSubject))
		}
		bodyLen := 0
		bodyTrimLen := 0
		if step.TemplateBody != nil {
			bodyLen = len(*step.TemplateBody)
			bodyTrimLen = len(strings.TrimSpace(*step.TemplateBody))
		}
		m.log.Info("workflow step matched",
			"orgId", orgID,
			"leadId", leadID,
			"workflowId", resolved.Workflow.ID,
			"stepId", step.ID,
			"resolutionSource", resolved.ResolutionSource,
			"trigger", trigger,
			"channel", channel,
			"audience", audience,
			"enabled", step.Enabled,
			"delayMinutes", step.DelayMinutes,
			"templateSubjectNil", step.TemplateSubject == nil,
			"templateSubjectLen", subjectLen,
			"templateSubjectTrimLen", subjectTrimLen,
			"templateBodyNil", step.TemplateBody == nil,
			"templateBodyLen", bodyLen,
			"templateBodyTrimLen", bodyTrimLen,
		)
		return &workflowRule{
			Enabled:         step.Enabled,
			DelayMinutes:    step.DelayMinutes,
			TemplateSubject: step.TemplateSubject,
			TemplateText:    step.TemplateBody,
		}
	}

	m.log.Debug("resolved workflow has no matching step", "orgId", orgID, "leadId", leadID, "workflowId", resolved.Workflow.ID, "trigger", trigger, "channel", channel, "audience", audience)
	return nil
}

func (m *Module) enqueueWorkflowSteps(ctx context.Context, steps []repository.WorkflowStep, execCtx workflowStepExecutionContext) error {
	if m.notificationOutbox == nil {
		m.log.Debug("notification outbox not configured; enqueue skipped", "orgId", execCtx.OrgID, "trigger", execCtx.Trigger)
		return nil
	}
	if len(steps) == 0 {
		m.log.Debug("no workflow steps provided", "orgId", execCtx.OrgID, "trigger", execCtx.Trigger)
		return nil
	}

	sorted := append([]repository.WorkflowStep(nil), steps...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].StepOrder == sorted[j].StepOrder {
			return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
		}
		return sorted[i].StepOrder < sorted[j].StepOrder
	})

	for _, step := range sorted {
		if !step.Enabled {
			m.log.Debug("skipping disabled workflow step", "orgId", execCtx.OrgID, "stepId", step.ID, "trigger", execCtx.Trigger)
			continue
		}
		if err := m.enqueueSingleWorkflowStep(ctx, step, execCtx); err != nil {
			return err
		}
	}
	m.log.Info("workflow steps enqueued", "orgId", execCtx.OrgID, "trigger", execCtx.Trigger, "stepCount", len(sorted))

	return nil
}

func (m *Module) enqueueSingleWorkflowStep(ctx context.Context, step repository.WorkflowStep, execCtx workflowStepExecutionContext) error {
	runAt := time.Now().UTC().Add(time.Duration(step.DelayMinutes) * time.Minute)
	vars := buildWorkflowStepVariables(execCtx)

	body, err := renderStepTemplate(step.TemplateBody, vars)
	if err != nil {
		return err
	}

	channel := strings.ToLower(strings.TrimSpace(step.Channel))
	summary := defaultName(strings.TrimSpace(execCtx.DefaultSummary), "Workflow bericht ingepland")
	actorType := defaultName(strings.TrimSpace(execCtx.DefaultActor), "System")
	actorName := defaultName(strings.TrimSpace(execCtx.DefaultOrigin), workflowEngineActorName)
	audience := defaultName(strings.TrimSpace(step.Audience), "lead")
	category := defaultName(strings.TrimSpace(execCtx.Trigger), "workflow_step")
	dispatchCtx := workflowStepDispatchContext{
		Step:      step,
		Exec:      execCtx,
		RunAt:     runAt,
		Body:      body,
		Category:  category,
		Audience:  audience,
		Summary:   summary,
		ActorType: actorType,
		ActorName: actorName,
	}

	switch channel {
	case "whatsapp":
		return m.enqueueWhatsAppWorkflowStep(ctx, dispatchCtx)
	case "email":
		return m.enqueueEmailWorkflowStep(ctx, vars, dispatchCtx)
	default:
		m.log.Warn("unsupported workflow channel; skipping step", "orgId", execCtx.OrgID, "channel", channel, "trigger", execCtx.Trigger, "stepId", step.ID)
		return nil
	}
}

func (m *Module) enqueueWhatsAppWorkflowStep(ctx context.Context, dispatchCtx workflowStepDispatchContext) error {
	message := normalizeWhatsAppMessage(dispatchCtx.Body)
	if strings.TrimSpace(message) == "" {
		m.log.Debug("workflow whatsapp step body empty; skipping", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "stepId", dispatchCtx.Step.ID)
		return nil
	}

	phones := resolveWorkflowStepPhoneRecipients(dispatchCtx.Step.RecipientConfig, dispatchCtx.Exec)
	if len(phones) == 0 {
		m.log.Debug("workflow whatsapp step has no recipients", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "stepId", dispatchCtx.Step.ID)
		return nil
	}
	for _, phoneNumber := range phones {
		payload := whatsAppSendOutboxPayload{
			OrgID:       dispatchCtx.Exec.OrgID.String(),
			LeadID:      ptrUUIDString(dispatchCtx.Exec.LeadID),
			ServiceID:   ptrUUIDString(dispatchCtx.Exec.ServiceID),
			PhoneNumber: phoneNumber,
			Message:     message,
			Category:    dispatchCtx.Category,
			Audience:    dispatchCtx.Audience,
			Summary:     dispatchCtx.Summary,
			ActorType:   dispatchCtx.ActorType,
			ActorName:   dispatchCtx.ActorName,
		}
		rec, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID:  dispatchCtx.Exec.OrgID,
			LeadID:    dispatchCtx.Exec.LeadID,
			ServiceID: dispatchCtx.Exec.ServiceID,
			Kind:      "whatsapp",
			Template:  "whatsapp_send",
			Payload:   payload,
			RunAt:     dispatchCtx.RunAt,
		})
		if err != nil {
			return err
		}
		m.log.Info("outbox message enqueued", "outboxId", rec.String(), "kind", "whatsapp", "template", "whatsapp_send", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "runAt", dispatchCtx.RunAt)
	}

	return nil
}

func (m *Module) enqueueEmailWorkflowStep(
	ctx context.Context,
	vars map[string]any,
	dispatchCtx workflowStepDispatchContext,
) error {
	subject, err := renderStepTemplate(dispatchCtx.Step.TemplateSubject, vars)
	if err != nil {
		return err
	}
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(dispatchCtx.Body) == "" {
		m.log.Debug("workflow email step missing subject/body; skipping", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "stepId", dispatchCtx.Step.ID)
		return nil
	}

	emails := resolveWorkflowStepEmailRecipients(dispatchCtx.Step.RecipientConfig, dispatchCtx.Exec)
	if len(emails) == 0 {
		m.log.Debug("workflow email step has no recipients", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "stepId", dispatchCtx.Step.ID)
		return nil
	}
	for _, toEmail := range emails {
		attachments := m.buildEmailAttachmentSpecs(dispatchCtx)
		payload := emailSendOutboxPayload{
			OrgID:       dispatchCtx.Exec.OrgID.String(),
			ToEmail:     toEmail,
			Subject:     subject,
			BodyHTML:    dispatchCtx.Body,
			LeadID:      ptrUUIDString(dispatchCtx.Exec.LeadID),
			ServiceID:   ptrUUIDString(dispatchCtx.Exec.ServiceID),
			Attachments: attachments,
		}
		rec, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID:  dispatchCtx.Exec.OrgID,
			LeadID:    dispatchCtx.Exec.LeadID,
			ServiceID: dispatchCtx.Exec.ServiceID,
			Kind:      "email",
			Template:  "email_send",
			Payload:   payload,
			RunAt:     dispatchCtx.RunAt,
		})
		if err != nil {
			return err
		}
		m.log.Info("outbox message enqueued", "outboxId", rec.String(), "kind", "email", "template", "email_send", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "runAt", dispatchCtx.RunAt)
	}

	return nil
}

func buildWorkflowStepVariables(execCtx workflowStepExecutionContext) map[string]any {
	vars := map[string]any{
		"lead": map[string]any{
			"name":        "",
			"firstName":   "",
			"lastName":    "",
			"phone":       execCtx.LeadPhone,
			"email":       execCtx.LeadEmail,
			"address":     "",
			"street":      "",
			"houseNumber": "",
			"zipCode":     "",
			"city":        "",
			"serviceType": "",
		},
		"partner": map[string]any{
			"name":  "",
			"phone": execCtx.PartnerPhone,
			"email": execCtx.PartnerEmail,
		},
		"org": map[string]any{
			"name": "",
		},
		"quote": map[string]any{
			"number":      "",
			"previewUrl":  "",
			"downloadUrl": "",
		},
		"links": map[string]any{
			"track": "",
		},
		"appointment": map[string]any{
			"date": "",
			"time": "",
		},
		"offer": map[string]any{
			"id": "",
		},
	}

	return mergeWorkflowTemplateVars(vars, execCtx.Variables)
}

func mergeWorkflowTemplateVars(base map[string]any, overrides map[string]any) map[string]any {
	if len(overrides) == 0 {
		return base
	}
	for key, value := range overrides {
		if value == nil {
			continue
		}
		if merged, ok := mergeWorkflowTemplateNestedMap(base[key], value); ok {
			base[key] = merged
			continue
		}
		base[key] = value
	}

	return base
}

func mergeWorkflowTemplateNestedMap(baseValue any, overrideValue any) (map[string]any, bool) {
	baseMap, baseIsMap := baseValue.(map[string]any)
	overrideMap, overrideIsMap := overrideValue.(map[string]any)
	if !baseIsMap || !overrideIsMap {
		return nil, false
	}

	merged := make(map[string]any, len(baseMap)+len(overrideMap))
	for nestedKey, nestedValue := range baseMap {
		merged[nestedKey] = nestedValue
	}
	for nestedKey, nestedValue := range overrideMap {
		if nestedValue == nil {
			continue
		}
		merged[nestedKey] = nestedValue
	}

	return merged, true
}

func renderWorkflowTemplateTextWithError(rule *workflowRule, vars map[string]any) (string, error) {
	if rule == nil || rule.TemplateText == nil {
		return "", nil
	}
	return renderStepTemplate(rule.TemplateText, vars)
}

func renderWorkflowTemplateSubjectWithError(rule *workflowRule, vars map[string]any) (string, error) {
	if rule == nil || rule.TemplateSubject == nil {
		return "", nil
	}
	return renderStepTemplate(rule.TemplateSubject, vars)
}

func resolveWorkflowStepPhoneRecipients(config map[string]any, execCtx workflowStepExecutionContext) []string {
	recipients := make([]string, 0)
	if getBoolFromConfig(config, "includeLeadContact") && strings.TrimSpace(execCtx.LeadPhone) != "" {
		recipients = append(recipients, execCtx.LeadPhone)
	}
	if getBoolFromConfig(config, "includePartner") && strings.TrimSpace(execCtx.PartnerPhone) != "" {
		recipients = append(recipients, execCtx.PartnerPhone)
	}
	recipients = append(recipients, getStringSliceFromConfig(config, "customPhones")...)
	return uniqueStrings(recipients)
}

func resolveWorkflowStepEmailRecipients(config map[string]any, execCtx workflowStepExecutionContext) []string {
	recipients := make([]string, 0)
	if getBoolFromConfig(config, "includeLeadContact") && strings.TrimSpace(execCtx.LeadEmail) != "" {
		recipients = append(recipients, execCtx.LeadEmail)
	}
	if getBoolFromConfig(config, "includePartner") && strings.TrimSpace(execCtx.PartnerEmail) != "" {
		recipients = append(recipients, execCtx.PartnerEmail)
	}
	recipients = append(recipients, getStringSliceFromConfig(config, "customEmails")...)
	return uniqueStrings(recipients)
}

func (m *Module) dispatchJobCompletedWorkflows(ctx context.Context, e events.PipelineStageChanged) {
	details := m.resolveLeadDetails(ctx, e.LeadID, e.TenantID)
	orgName := m.resolveOrganizationName(ctx, e.TenantID)

	// Load org settings to get the review URL.
	var reviewURL string
	if m.settingsReader != nil {
		settings, err := m.settingsReader.GetOrganizationSettings(ctx, e.TenantID)
		if err != nil {
			m.log.Warn("job_completed: failed to load org settings", "orgId", e.TenantID, "error", err)
		} else if settings.ReviewURL != nil {
			reviewURL = *settings.ReviewURL
		}
	}

	leadName := "klant"
	leadPhone := ""
	leadEmail := ""
	if details != nil {
		if n := strings.TrimSpace(details.FirstName + " " + details.LastName); n != "" {
			leadName = n
		}
		leadPhone = details.Phone
		leadEmail = details.Email
	}

	templateVars := map[string]any{
		"lead": map[string]any{"name": leadName, "phone": leadPhone, "email": leadEmail},
		"org":  map[string]any{"name": orgName, "reviewUrl": reviewURL},
	}
	enrichLeadVars(templateVars, details)

	serviceID := e.LeadServiceID

	whatsAppRule := m.resolveWorkflowRule(ctx, e.TenantID, e.LeadID, "job_completed", "whatsapp", "lead", nil)
	m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         whatsAppRule,
		OrgID:        e.TenantID,
		LeadID:       &e.LeadID,
		ServiceID:    &serviceID,
		LeadPhone:    leadPhone,
		Trigger:      "job_completed",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("WhatsApp review-verzoek verstuurd naar %s", leadName),
		FallbackNote: "failed to enqueue job_completed lead whatsapp workflow",
	})

	emailRule := m.resolveWorkflowRule(ctx, e.TenantID, e.LeadID, "job_completed", "email", "lead", nil)
	m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         emailRule,
		OrgID:        e.TenantID,
		LeadID:       &e.LeadID,
		ServiceID:    &serviceID,
		LeadEmail:    leadEmail,
		Trigger:      "job_completed",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email review-verzoek verstuurd naar %s", leadName),
		FallbackNote: "failed to enqueue job_completed lead email workflow",
	})

	m.log.Info("job_completed workflows dispatched", "leadId", e.LeadID, "orgId", e.TenantID)
}
