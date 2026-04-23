package notification

import (
	"context"
	"fmt"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/platform/timekit"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (m *Module) handleAppointmentCreated(ctx context.Context, e events.AppointmentCreated) error {
	params := appointmentWhatsAppParams{
		OrgID:         e.OrganizationID,
		LeadID:        e.LeadID,
		ServiceID:     e.LeadServiceID,
		Type:          e.Type,
		ConsumerPhone: e.ConsumerPhone,
		ConsumerEmail: e.ConsumerEmail,
		ConsumerName:  e.ConsumerName,
		StartTime:     e.StartTime,
		Location:      e.Location,
		Trigger:       "appointment_created",
		Category:      "appointment_created",
		SummaryFmt:    "WhatsApp afspraakbevestiging verstuurd naar %s",
	}

	if err := m.handleAppointmentWhatsApp(ctx, params); err != nil {
		return err
	}

	return m.handleAppointmentEmail(ctx, params)
}

func (m *Module) handleAppointmentReminderDue(ctx context.Context, e events.AppointmentReminderDue) error {
	params := appointmentWhatsAppParams{
		OrgID:         e.OrganizationID,
		LeadID:        e.LeadID,
		ServiceID:     e.LeadServiceID,
		Type:          e.Type,
		ConsumerPhone: e.ConsumerPhone,
		ConsumerEmail: e.ConsumerEmail,
		ConsumerName:  e.ConsumerName,
		StartTime:     e.StartTime,
		Location:      e.Location,
		Trigger:       "appointment_reminder",
		Category:      "appointment_reminder",
		SummaryFmt:    "WhatsApp afspraakherinnering verstuurd naar %s",
	}

	if err := m.handleAppointmentWhatsApp(ctx, params); err != nil {
		return err
	}

	return m.handleAppointmentEmail(ctx, params)
}

type appointmentWhatsAppParams struct {
	OrgID         uuid.UUID
	LeadID        *uuid.UUID
	ServiceID     *uuid.UUID
	Type          string
	ConsumerPhone string
	ConsumerEmail string
	ConsumerName  string
	StartTime     time.Time
	Location      string
	Trigger       string
	Category      string
	SummaryFmt    string
}

func (m *Module) handleAppointmentWhatsApp(ctx context.Context, p appointmentWhatsAppParams) error {
	if p.Type != "lead_visit" || strings.TrimSpace(p.ConsumerPhone) == "" || p.LeadID == nil {
		return nil
	}
	if !m.isLeadWhatsAppOptedIn(ctx, *p.LeadID, p.OrgID) {
		return nil
	}

	rule := m.resolveWorkflowRule(ctx, p.OrgID, *p.LeadID, p.Trigger, "whatsapp", "lead", nil)
	if rule != nil && !rule.Enabled {
		return nil
	}

	name := defaultName(strings.TrimSpace(p.ConsumerName), "klant")
	nlLoc := timekit.ResolveLocation("Europe/Amsterdam")
	localStart := p.StartTime.In(nlLoc)
	dateStr := localStart.Format("02-01-2006")
	timeStr := localStart.Format("15:04")
	details := m.resolveLeadDetails(ctx, *p.LeadID, p.OrgID)
	orgName := defaultName(strings.TrimSpace(m.resolveOrganizationName(ctx, p.OrgID)), defaultOrgNameFallback)
	templateVars := buildAppointmentTemplateVars(name, p.ConsumerPhone, p.ConsumerEmail, dateStr, timeStr, strings.TrimSpace(p.Location), orgName)
	enrichLeadVars(templateVars, details)
	bodyText, err := renderWorkflowTemplateTextWithError(rule, templateVars)
	if err != nil {
		m.log.Warn("workflow whatsapp template render failed", "orgId", p.OrgID, "trigger", p.Trigger, "error", err)
		return nil
	}
	if strings.TrimSpace(bodyText) == "" {
		return nil
	}
	_ = m.enqueueAppointmentOutbox(ctx, p, rule, bodyText, name)
	return nil
}

func (m *Module) handleAppointmentEmail(ctx context.Context, p appointmentWhatsAppParams) error {
	if p.Type != "lead_visit" || strings.TrimSpace(p.ConsumerEmail) == "" || p.LeadID == nil {
		return nil
	}

	rule := m.resolveWorkflowRule(ctx, p.OrgID, *p.LeadID, p.Trigger, "email", "lead", nil)
	if rule != nil && !rule.Enabled {
		return nil
	}

	name := defaultName(strings.TrimSpace(p.ConsumerName), "klant")
	nlLoc := timekit.ResolveLocation("Europe/Amsterdam")
	localStart := p.StartTime.In(nlLoc)
	dateStr := localStart.Format("02-01-2006")
	timeStr := localStart.Format("15:04")
	details := m.resolveLeadDetails(ctx, *p.LeadID, p.OrgID)
	orgName := defaultName(strings.TrimSpace(m.resolveOrganizationName(ctx, p.OrgID)), defaultOrgNameFallback)
	templateVars := buildAppointmentTemplateVars(name, p.ConsumerPhone, p.ConsumerEmail, dateStr, timeStr, strings.TrimSpace(p.Location), orgName)
	enrichLeadVars(templateVars, details)

	_ = m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        p.OrgID,
		LeadID:       p.LeadID,
		ServiceID:    p.ServiceID,
		LeadEmail:    p.ConsumerEmail,
		Trigger:      p.Trigger,
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email afspraakbericht verstuurd naar %s", name),
		FallbackNote: fmt.Sprintf("failed to enqueue %s lead email workflow", p.Trigger),
	})

	return nil
}

func buildAppointmentTemplateVars(consumerName, consumerPhone, consumerEmail, date, timeText, location, orgName string) map[string]any {
	return map[string]any{
		"lead": map[string]any{
			"name":  consumerName,
			"phone": consumerPhone,
			"email": consumerEmail,
		},
		"appointment": map[string]any{
			"date":     date,
			"time":     timeText,
			"location": location,
		},
		"org": map[string]any{
			"name": orgName,
		},
	}
}

func (m *Module) enqueueAppointmentOutbox(ctx context.Context, p appointmentWhatsAppParams, rule *workflowRule, message, name string) bool {
	if m.notificationOutbox == nil {
		return false
	}

	delayMinutes := 0
	if rule != nil {
		delayMinutes = rule.DelayMinutes
	}
	messageText := message
	steps := []repository.WorkflowStep{{
		Enabled:      true,
		Channel:      "whatsapp",
		Audience:     "lead",
		DelayMinutes: delayMinutes,
		TemplateBody: &messageText,
		RecipientConfig: map[string]any{
			"includeLeadContact": true,
		},
	}}

	err := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadPhone:      p.ConsumerPhone,
		Trigger:        p.Category,
		DefaultSummary: fmt.Sprintf(p.SummaryFmt, name),
		DefaultActor:   "System",
		DefaultOrigin:  "Portal",
	})
	return err == nil
}
