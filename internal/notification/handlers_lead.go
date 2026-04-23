package notification

import (
	"context"
	"errors"
	"fmt"
	"portal_final_backend/internal/events"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/inapp"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"strings"

	"github.com/google/uuid"
)

// LeadWhatsAppReader checks if a lead is opted in for WhatsApp messages.
type LeadWhatsAppReader interface {
	IsWhatsAppOptedIn(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error)
}

// LeadAssigneeReader fetches the currently assigned agent for a lead.
type LeadAssigneeReader interface {
	GetAssignedAgentID(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) (*uuid.UUID, error)
}

// LeadTimelineEventParams describes a lead timeline event payload.
type LeadTimelineEventParams struct {
	LeadID     uuid.UUID
	ServiceID  *uuid.UUID
	OrgID      uuid.UUID
	ActorType  string
	ActorName  string
	EventType  string
	Title      string
	Summary    *string
	Metadata   map[string]any
	Visibility string
}

// LeadTimelineWriter persists lead timeline events.
type LeadTimelineWriter interface {
	CreateTimelineEvent(ctx context.Context, params LeadTimelineEventParams) error
}

type SendLeadWhatsAppParams struct {
	OrgID       uuid.UUID
	LeadID      uuid.UUID
	ServiceID   *uuid.UUID
	PhoneNumber string
	Message     string
	Category    string
	Audience    string
	Summary     string
	ActorType   string
	ActorName   string
	Metadata    map[string]any
}

// leadDetails holds resolved lead contact and address fields.
type leadDetails struct {
	FirstName   string
	LastName    string
	Phone       string
	Email       string
	Street      string
	HouseNumber string
	ZipCode     string
	City        string
	ServiceType string
	PublicToken string
}

// enrichLeadVars adds first name, last name, address, city, zip code and service type
// into an existing "lead" template variable map. Creates the map if nil.
func enrichLeadVars(vars map[string]any, d *leadDetails) {
	if d == nil {
		return
	}
	leadMap, ok := vars["lead"].(map[string]any)
	if !ok {
		leadMap = map[string]any{}
		vars["lead"] = leadMap
	}
	leadMap["firstName"] = strings.TrimSpace(d.FirstName)
	leadMap["lastName"] = strings.TrimSpace(d.LastName)
	leadMap["address"] = strings.TrimSpace(d.Street + " " + d.HouseNumber)
	leadMap["street"] = strings.TrimSpace(d.Street)
	leadMap["houseNumber"] = strings.TrimSpace(d.HouseNumber)
	leadMap["zipCode"] = strings.TrimSpace(d.ZipCode)
	leadMap["city"] = strings.TrimSpace(d.City)
	leadMap["serviceType"] = strings.TrimSpace(d.ServiceType)
}

func buildLeadAddressFromWorkflowVars(vars map[string]any) string {
	leadMap, ok := vars["lead"].(map[string]any)
	if !ok {
		return ""
	}
	if address := strings.TrimSpace(stringFromMap(leadMap, "address")); address != "" {
		return address
	}
	parts := []string{
		strings.TrimSpace(strings.Join([]string{stringFromMap(leadMap, "street"), stringFromMap(leadMap, "houseNumber")}, " ")),
		strings.TrimSpace(strings.Join([]string{stringFromMap(leadMap, "zipCode"), stringFromMap(leadMap, "city")}, " ")),
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, ", ")
}

func (m *Module) handleLeadCreated(ctx context.Context, e events.LeadCreated) error {
	m.log.Info("processing lead created notification", "leadId", e.LeadID, "orgId", e.TenantID, "source", strings.TrimSpace(e.Source), "leadServiceId", e.LeadServiceID)
	if strings.EqualFold(strings.TrimSpace(e.Source), "quote_flow") {
		m.log.Info("lead created from quote flow, skipping welcome message", "leadId", e.LeadID)
		return nil
	}
	m.log.Info("lead welcome eligibility",
		"leadId", e.LeadID,
		"orgId", e.TenantID,
		"whatsAppOptedIn", e.WhatsAppOptedIn,
		"hasPhone", strings.TrimSpace(e.ConsumerPhone) != "",
		"hasEmail", strings.TrimSpace(e.ConsumerEmail) != "",
	)

	consumerName := defaultName(strings.TrimSpace(e.ConsumerName), "daar")
	source := strings.TrimSpace(e.Source)
	var leadSource *string
	if source != "" {
		leadSource = &source
	}
	orgName := strings.TrimSpace(m.resolveOrganizationName(ctx, e.TenantID))
	details := m.resolveLeadDetails(ctx, e.LeadID, e.TenantID)
	templateVars := map[string]any{
		"lead": map[string]any{
			"name":   consumerName,
			"phone":  e.ConsumerPhone,
			"email":  e.ConsumerEmail,
			"source": source,
		},
		"org": map[string]any{
			"name": defaultName(orgName, defaultOrgNameFallback),
		},
		"links": map[string]any{
			"track": m.buildLeadTrackLink(e.PublicToken),
		},
	}
	enrichLeadVars(templateVars, details)

	whatsAppRule := m.resolveWorkflowRule(ctx, e.TenantID, e.LeadID, "lead_welcome", "whatsapp", "lead", leadSource)
	whatsAppEligible := whatsAppRule != nil && whatsAppRule.Enabled && e.WhatsAppOptedIn && strings.TrimSpace(e.ConsumerPhone) != ""
	m.log.Info("lead welcome whatsapp rule evaluation",
		"leadId", e.LeadID,
		"orgId", e.TenantID,
		"ruleFound", whatsAppRule != nil,
		"ruleEnabled", whatsAppRule != nil && whatsAppRule.Enabled,
		"eventOptedIn", e.WhatsAppOptedIn,
		"hasPhone", strings.TrimSpace(e.ConsumerPhone) != "",
		"eligible", whatsAppEligible,
	)
	whatsAppDispatched := false
	if whatsAppRule != nil && whatsAppRule.Enabled && e.WhatsAppOptedIn && strings.TrimSpace(e.ConsumerPhone) != "" {
		whatsAppDispatched = m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
			Rule:         whatsAppRule,
			OrgID:        e.TenantID,
			LeadID:       &e.LeadID,
			ServiceID:    &e.LeadServiceID,
			LeadPhone:    e.ConsumerPhone,
			Trigger:      "lead_welcome",
			TemplateVars: templateVars,
			Summary:      fmt.Sprintf(leadWelcomeSummaryFmt, consumerName),
			FallbackNote: "failed to enqueue lead_welcome lead whatsapp workflow",
		})
	}
	m.log.Info("lead welcome whatsapp dispatch outcome",
		"leadId", e.LeadID,
		"orgId", e.TenantID,
		"attempted", whatsAppEligible,
		"dispatched", whatsAppDispatched,
	)

	emailRule := m.resolveWorkflowRule(ctx, e.TenantID, e.LeadID, "lead_welcome", "email", "lead", leadSource)
	emailEligible := emailRule != nil && emailRule.Enabled && strings.TrimSpace(e.ConsumerEmail) != ""
	m.log.Info("lead welcome email rule evaluation",
		"leadId", e.LeadID,
		"orgId", e.TenantID,
		"ruleFound", emailRule != nil,
		"ruleEnabled", emailRule != nil && emailRule.Enabled,
		"hasEmail", strings.TrimSpace(e.ConsumerEmail) != "",
		"eligible", emailEligible,
	)
	emailDispatched := false
	if emailRule != nil && emailRule.Enabled && strings.TrimSpace(e.ConsumerEmail) != "" {
		emailDispatched = m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
			Rule:         emailRule,
			OrgID:        e.TenantID,
			LeadID:       &e.LeadID,
			ServiceID:    &e.LeadServiceID,
			LeadEmail:    e.ConsumerEmail,
			Trigger:      "lead_welcome",
			TemplateVars: templateVars,
			Summary:      fmt.Sprintf("Email welkomstbericht verstuurd naar %s", consumerName),
			FallbackNote: "failed to enqueue lead_welcome lead email workflow",
		})
	}
	m.log.Info("lead welcome email dispatch outcome",
		"leadId", e.LeadID,
		"orgId", e.TenantID,
		"attempted", emailEligible,
		"dispatched", emailDispatched,
	)

	return nil
}

func (m *Module) handleWhatsAppOutboxLeadLookupError(ctx context.Context, rec notificationoutbox.Record, leadID, orgID uuid.UUID, err error) error {
	if errors.Is(err, leadrepo.ErrNotFound) {
		m.log.Info(
			"lead not found for whatsapp outbox; marking succeeded",
			"outboxId", rec.ID.String(),
			"leadId", leadID,
			"orgId", orgID,
		)
		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	m.log.Warn(
		"failed to resolve lead whatsapp opt-in for outbox; will retry",
		"outboxId", rec.ID.String(),
		"leadId", leadID,
		"orgId", orgID,
		"error", err,
	)
	return err
}

func (m *Module) buildLeadTrackLink(publicToken string) string {
	if strings.TrimSpace(publicToken) == "" {
		return ""
	}
	base := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/track/%s", base, publicToken)
}

func (m *Module) handleLeadDataChanged(ctx context.Context, e events.LeadDataChanged) error {
	var eventType sse.EventType
	var message string
	shouldNotifyAgent := false

	switch e.Source {
	case "customer_preferences":
		eventType = sse.EventLeadPreferencesUpdated
		message = "Klant heeft voorkeuren bijgewerkt"
	case "customer_portal_update":
		eventType = sse.EventLeadInfoAdded
		message = "Klant heeft extra info toegevoegd"
		shouldNotifyAgent = true
	case "customer_portal_upload":
		eventType = sse.EventLeadAttachmentUploaded
		message = "Klant heeft bestanden geupload"
		shouldNotifyAgent = true
	case "customer_portal_delete":
		eventType = sse.EventLeadAttachmentDeleted
		message = "Klant heeft een bestand verwijderd"
	case "appointment_request":
		eventType = sse.EventLeadAppointmentRequested
		message = "Klant heeft een inspectie aangevraagd"
	default:
		return nil
	}

	if m.sse != nil {
		m.sse.PublishToOrganization(e.TenantID, sse.Event{
			Type:      eventType,
			LeadID:    e.LeadID,
			ServiceID: e.LeadServiceID,
			Message:   message,
			Data: map[string]interface{}{
				"source": e.Source,
			},
		})
	}

	if shouldNotifyAgent {
		leadID := e.LeadID
		content := "Nieuwe informatie toegevoegd door de klant voor lead."
		if e.Source == "customer_portal_upload" {
			content = "Nieuwe foto's/informatie geupload door de klant voor lead."
		}
		m.sendToAgentOrAdmins(ctx, e.TenantID, e.LeadID, inapp.SendParams{
			Title:        "Nieuwe informatie van klant",
			Content:      content,
			ResourceID:   &leadID,
			ResourceType: "lead",
			Category:     "info",
		})
	}

	return nil
}

func (m *Module) handleLeadAssigned(ctx context.Context, e events.LeadAssigned) error {
	if m.inAppService == nil || e.NewAgent == nil {
		return nil
	}

	_ = m.inAppService.Send(ctx, inapp.SendParams{
		OrgID:        e.TenantID,
		UserID:       *e.NewAgent,
		Title:        "Nieuwe lead toegewezen",
		Content:      "Je bent toegewezen aan een lead.",
		ResourceID:   &e.LeadID,
		ResourceType: "lead",
		Category:     "info",
	})

	return nil
}

func (m *Module) isLeadWhatsAppOptedIn(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) bool {
	if m.leadWhatsAppReader == nil {
		return true
	}
	optedIn, err := m.leadWhatsAppReader.IsWhatsAppOptedIn(ctx, leadID, organizationID)
	if err != nil {
		m.log.Warn("failed to resolve lead whatsapp opt-in", "leadId", leadID, "orgId", organizationID, "error", err)
		return false
	}
	if !optedIn {
		m.log.Info("whatsapp disabled for lead, skipping message", "leadId", leadID, "orgId", organizationID)
	}
	return optedIn
}
