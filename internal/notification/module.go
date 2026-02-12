// Package notification provides event handlers for sending notifications
// (emails, SMS, push, etc.) in response to domain events.
// This module subscribes to events and inverts the dependency: domain modules
// no longer need to know about email providers or templates.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/smtpcrypto"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

// QuoteActivityWriter persists activity log entries for quotes.
type QuoteActivityWriter interface {
	CreateActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) error
}

// PartnerOfferTimelineWriter writes partner-offer events into the leads timeline.
type PartnerOfferTimelineWriter interface {
	WriteOfferEvent(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, orgID uuid.UUID, actorType, actorName, eventType, title string, summary *string, metadata map[string]any) error
}

// WhatsAppSender sends WhatsApp messages.
type WhatsAppSender interface {
	SendMessage(ctx context.Context, deviceID string, phoneNumber string, message string) error
}

// OrganizationSettingsReader provides org-level settings for notifications.
type OrganizationSettingsReader interface {
	GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (repository.OrganizationSettings, error)
}

type WorkflowResolver interface {
	ResolveLeadWorkflow(ctx context.Context, input identityservice.ResolveLeadWorkflowInput) (identityservice.ResolveLeadWorkflowResult, error)
}

// LeadWhatsAppReader checks if a lead is opted in for WhatsApp messages.
type LeadWhatsAppReader interface {
	IsWhatsAppOptedIn(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (bool, error)
}

// LeadTimelineEventParams describes a lead timeline event payload.
type LeadTimelineEventParams struct {
	LeadID    uuid.UUID
	ServiceID *uuid.UUID
	OrgID     uuid.UUID
	ActorType string
	ActorName string
	EventType string
	Title     string
	Summary   *string
	Metadata  map[string]any
}

// LeadTimelineWriter persists lead timeline events.
type LeadTimelineWriter interface {
	CreateTimelineEvent(ctx context.Context, params LeadTimelineEventParams) error
}

// cachedSender holds a resolved email.Sender with a TTL for cache expiry.
type cachedSender struct {
	sender    email.Sender
	expiresAt time.Time
}

// Module handles all notification-related event subscriptions.
type Module struct {
	sender             email.Sender
	cfg                config.NotificationConfig
	log                *logger.Logger
	sse                *sse.Service
	actWriter          QuoteActivityWriter
	offerTimeline      PartnerOfferTimelineWriter
	whatsapp           WhatsAppSender
	leadTimeline       LeadTimelineWriter
	settingsReader     OrganizationSettingsReader
	workflowResolver   WorkflowResolver
	leadWhatsAppReader LeadWhatsAppReader
	notificationOutbox *notificationoutbox.Repository
	smtpEncryptionKey  []byte
	senderCache        sync.Map // map[uuid.UUID]cachedSender
}

// New creates a new notification module.
func New(sender email.Sender, cfg config.NotificationConfig, log *logger.Logger) *Module {
	return &Module{
		sender: sender,
		cfg:    cfg,
		log:    log,
	}
}

// SetSSE injects the SSE service so quote events can be pushed to agents.
func (m *Module) SetSSE(s *sse.Service) { m.sse = s }

// SetQuoteActivityWriter injects the writer for persisting quote activity log entries.
func (m *Module) SetQuoteActivityWriter(w QuoteActivityWriter) { m.actWriter = w }

// SetOfferTimelineWriter injects the writer for persisting partner-offer timeline events.
func (m *Module) SetOfferTimelineWriter(w PartnerOfferTimelineWriter) {
	m.offerTimeline = w
}

// SetWhatsAppSender injects the WhatsApp sender.
func (m *Module) SetWhatsAppSender(sender WhatsAppSender) { m.whatsapp = sender }

// SetOrganizationSettingsReader injects org settings reader for WhatsApp device resolution.
func (m *Module) SetOrganizationSettingsReader(reader OrganizationSettingsReader) {
	m.settingsReader = reader
}

func (m *Module) SetWorkflowResolver(resolver WorkflowResolver) {
	m.workflowResolver = resolver
}

// SetLeadWhatsAppReader injects a reader for lead WhatsApp opt-in state.
func (m *Module) SetLeadWhatsAppReader(reader LeadWhatsAppReader) { m.leadWhatsAppReader = reader }

// SetLeadTimelineWriter injects the lead timeline writer.
func (m *Module) SetLeadTimelineWriter(writer LeadTimelineWriter) { m.leadTimeline = writer }

// SetSMTPEncryptionKey sets the AES key used to decrypt SMTP passwords from org settings.
func (m *Module) SetSMTPEncryptionKey(key []byte) { m.smtpEncryptionKey = key }

// SetNotificationOutbox injects the notification outbox repository.
func (m *Module) SetNotificationOutbox(repo *notificationoutbox.Repository) {
	m.notificationOutbox = repo
}

// resolveSender returns a tenant-specific SMTPSender if the organization has SMTP
// configured, falling back to the default (Brevo) sender. Results are cached with a
// 5-minute TTL to avoid repeated DB lookups and decryption.
func (m *Module) resolveSender(ctx context.Context, orgID uuid.UUID) email.Sender {
	// Check cache first.
	if cached, ok := m.senderCache.Load(orgID); ok {
		entry := cached.(cachedSender)
		if time.Now().Before(entry.expiresAt) {
			return entry.sender
		}
		m.senderCache.Delete(orgID)
	}

	// No cache hit — resolve from DB.
	if m.settingsReader == nil || len(m.smtpEncryptionKey) == 0 {
		return m.sender
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to fetch org settings for smtp", "error", err, "orgId", orgID)
		return m.sender
	}

	if settings.SMTPHost == nil || *settings.SMTPHost == "" {
		// No SMTP configured — cache the default sender so we don't query again soon.
		m.senderCache.Store(orgID, cachedSender{sender: m.sender, expiresAt: time.Now().Add(5 * time.Minute)})
		return m.sender
	}

	smtpSender, err := m.buildSMTPSender(settings)
	if err != nil {
		m.log.Error("failed to build smtp sender", "error", err, "orgId", orgID)
		return m.sender
	}

	m.senderCache.Store(orgID, cachedSender{sender: smtpSender, expiresAt: time.Now().Add(5 * time.Minute)})
	m.log.Info("resolved tenant smtp sender", "orgId", orgID, "host", *settings.SMTPHost)
	return smtpSender
}

// buildSMTPSender creates an SMTPSender from organization settings, decrypting the password.
func (m *Module) buildSMTPSender(settings repository.OrganizationSettings) (email.Sender, error) {
	password := ""
	if settings.SMTPPassword != nil && *settings.SMTPPassword != "" {
		decrypted, err := smtpcrypto.Decrypt(*settings.SMTPPassword, m.smtpEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt smtp password: %w", err)
		}
		password = decrypted
	}

	port := 587
	if settings.SMTPPort != nil {
		port = *settings.SMTPPort
	}

	return email.NewSMTPSender(
		*settings.SMTPHost,
		port,
		derefStr(settings.SMTPUsername),
		password,
		derefStr(settings.SMTPFromEmail),
		derefStr(settings.SMTPFromName),
	), nil
}

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// InvalidateSMTPCache removes the cached sender for an organization, forcing
// re-resolution on the next email send. Called when SMTP settings are updated.
func (m *Module) InvalidateSMTPCache(orgID uuid.UUID) {
	m.senderCache.Delete(orgID)
}

// RegisterHandlers subscribes to all relevant domain events on the event bus.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {
	// Auth domain events
	bus.Subscribe(events.UserSignedUp{}.EventName(), m)
	bus.Subscribe(events.EmailVerificationRequested{}.EventName(), m)
	bus.Subscribe(events.PasswordResetRequested{}.EventName(), m)

	// Identity domain events
	bus.Subscribe(events.OrganizationInviteCreated{}.EventName(), m)
	// Partners domain events
	bus.Subscribe(events.PartnerInviteCreated{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferCreated{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferAccepted{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferRejected{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferExpired{}.EventName(), m)

	// Lead events
	bus.Subscribe(events.LeadCreated{}.EventName(), m)
	bus.Subscribe(events.LeadDataChanged{}.EventName(), m)
	bus.Subscribe(events.PipelineStageChanged{}.EventName(), m)

	// Quote domain events
	bus.Subscribe(events.QuoteSent{}.EventName(), m)
	bus.Subscribe(events.QuoteViewed{}.EventName(), m)
	bus.Subscribe(events.QuoteUpdatedByCustomer{}.EventName(), m)
	bus.Subscribe(events.QuoteAnnotated{}.EventName(), m)
	bus.Subscribe(events.QuoteAccepted{}.EventName(), m)
	bus.Subscribe(events.QuoteRejected{}.EventName(), m)

	// Appointment domain events
	bus.Subscribe(events.AppointmentCreated{}.EventName(), m)
	bus.Subscribe(events.AppointmentReminderDue{}.EventName(), m)
	bus.Subscribe(events.NotificationOutboxDue{}.EventName(), m)

	m.log.Info("notification module registered event handlers")
}

// Handle routes events to the appropriate handler method.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	switch e := event.(type) {
	case events.UserSignedUp:
		return m.handleUserSignedUp(ctx, e)
	case events.EmailVerificationRequested:
		return m.handleEmailVerificationRequested(ctx, e)
	case events.PasswordResetRequested:
		return m.handlePasswordResetRequested(ctx, e)
	case events.OrganizationInviteCreated:
		return m.handleOrganizationInviteCreated(ctx, e)
	case events.PartnerInviteCreated:
		return m.handlePartnerInviteCreated(ctx, e)
	case events.PartnerOfferCreated:
		return m.handlePartnerOfferCreated(ctx, e)
	case events.PartnerOfferAccepted:
		return m.handlePartnerOfferAccepted(ctx, e)
	case events.PartnerOfferRejected:
		return m.handlePartnerOfferRejected(ctx, e)
	case events.PartnerOfferExpired:
		return m.handlePartnerOfferExpired(ctx, e)
	case events.LeadCreated:
		return m.handleLeadCreated(ctx, e)
	case events.LeadDataChanged:
		return m.handleLeadDataChanged(ctx, e)
	case events.PipelineStageChanged:
		return m.handlePipelineStageChanged(ctx, e)
	// Quote events
	case events.QuoteSent:
		return m.handleQuoteSent(ctx, e)
	case events.QuoteViewed:
		return m.handleQuoteViewed(ctx, e)
	case events.QuoteUpdatedByCustomer:
		return m.handleQuoteUpdatedByCustomer(ctx, e)
	case events.QuoteAnnotated:
		return m.handleQuoteAnnotated(ctx, e)
	case events.QuoteAccepted:
		return m.handleQuoteAccepted(ctx, e)
	case events.QuoteRejected:
		return m.handleQuoteRejected(ctx, e)
	case events.AppointmentCreated:
		return m.handleAppointmentCreated(ctx, e)
	case events.AppointmentReminderDue:
		return m.handleAppointmentReminderDue(ctx, e)
	case events.NotificationOutboxDue:
		return m.handleNotificationOutboxDue(ctx, e)
	default:
		m.log.Warn("unhandled event type", "event", event.EventName())
		return nil
	}
}

type whatsAppSendOutboxPayload struct {
	OrgID       string         `json:"orgId"`
	LeadID      *string        `json:"leadId,omitempty"`
	ServiceID   *string        `json:"serviceId,omitempty"`
	PhoneNumber string         `json:"phoneNumber"`
	Message     string         `json:"message"`
	Category    string         `json:"category"`
	Audience    string         `json:"audience"`
	Summary     string         `json:"summary"`
	ActorType   string         `json:"actorType"`
	ActorName   string         `json:"actorName"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type emailSendOutboxPayload struct {
	OrgID     string  `json:"orgId"`
	ToEmail   string  `json:"toEmail"`
	Subject   string  `json:"subject"`
	BodyHTML  string  `json:"bodyHtml"`
	LeadID    *string `json:"leadId,omitempty"`
	ServiceID *string `json:"serviceId,omitempty"`
}

type workflowRule struct {
	Enabled      bool
	DelayMinutes int
	TemplateText *string
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
		)
		return &workflowRule{
			Enabled:      step.Enabled,
			DelayMinutes: step.DelayMinutes,
			TemplateText: step.TemplateBody,
		}
	}

	m.log.Debug("resolved workflow has no matching step", "orgId", orgID, "leadId", leadID, "workflowId", resolved.Workflow.ID, "trigger", trigger, "channel", channel, "audience", audience)
	return nil
}

func renderTemplateText(tpl string, data map[string]any) (string, error) {
	if strings.Contains(tpl, "{{.") {
		return "", errors.New("legacy template syntax is not supported; use frontend syntax like {{lead.name}}")
	}

	normalizedTpl := normalizeFrontendTemplateSyntax(tpl)
	parsed, err := template.New("msg").Option("missingkey=zero").Parse(normalizedTpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := parsed.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

var frontendPlaceholderPattern = regexp.MustCompile(`{{\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s*}}`)

func normalizeFrontendTemplateSyntax(tpl string) string {
	return frontendPlaceholderPattern.ReplaceAllString(tpl, "{{.$1}}")
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
	if strings.TrimSpace(dispatchCtx.Body) == "" {
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
			Message:     dispatchCtx.Body,
			Category:    dispatchCtx.Category,
			Audience:    dispatchCtx.Audience,
			Summary:     dispatchCtx.Summary,
			ActorType:   dispatchCtx.ActorType,
			ActorName:   dispatchCtx.ActorName,
		}
		rec, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID: dispatchCtx.Exec.OrgID,
			Kind:     "whatsapp",
			Template: "whatsapp_send",
			Payload:  payload,
			RunAt:    dispatchCtx.RunAt,
		})
		if err != nil {
			return err
		}
		m.log.Info("outbox message enqueued", "outboxId", rec.ID, "kind", "whatsapp", "template", "whatsapp_send", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "runAt", dispatchCtx.RunAt)
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
		payload := emailSendOutboxPayload{
			OrgID:     dispatchCtx.Exec.OrgID.String(),
			ToEmail:   toEmail,
			Subject:   subject,
			BodyHTML:  dispatchCtx.Body,
			LeadID:    ptrUUIDString(dispatchCtx.Exec.LeadID),
			ServiceID: ptrUUIDString(dispatchCtx.Exec.ServiceID),
		}
		rec, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID: dispatchCtx.Exec.OrgID,
			Kind:     "email",
			Template: "email_send",
			Payload:  payload,
			RunAt:    dispatchCtx.RunAt,
		})
		if err != nil {
			return err
		}
		m.log.Info("outbox message enqueued", "outboxId", rec.ID, "kind", "email", "template", "email_send", "orgId", dispatchCtx.Exec.OrgID, "trigger", dispatchCtx.Exec.Trigger, "runAt", dispatchCtx.RunAt)
	}

	return nil
}

func buildWorkflowStepVariables(execCtx workflowStepExecutionContext) map[string]any {
	vars := map[string]any{
		"lead": map[string]any{
			"name":  "",
			"phone": execCtx.LeadPhone,
			"email": execCtx.LeadEmail,
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
			"number":     "",
			"previewUrl": "",
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

	if execCtx.Variables == nil {
		return vars
	}
	for key, value := range execCtx.Variables {
		vars[key] = value
	}

	return vars
}

func renderStepTemplate(raw *string, vars map[string]any) (string, error) {
	if raw == nil {
		return "", nil
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return "", nil
	}
	rendered, err := renderTemplateText(text, vars)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered), nil
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

func getBoolFromConfig(config map[string]any, key string) bool {
	if config == nil {
		return false
	}
	raw, ok := config[key]
	if !ok {
		return false
	}
	value, ok := raw.(bool)
	return ok && value
}

func getStringSliceFromConfig(config map[string]any, key string) []string {
	if config == nil {
		return nil
	}
	raw, ok := config[key]
	if !ok {
		return nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		text, ok := entry.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func (m *Module) handleUserSignedUp(ctx context.Context, e events.UserSignedUp) error {
	verifyURL := m.buildURL("/verify-email", e.VerifyToken)
	if err := m.sender.SendVerificationEmail(ctx, e.Email, verifyURL); err != nil {
		m.log.Error("failed to send verification email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("verification email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handleEmailVerificationRequested(ctx context.Context, e events.EmailVerificationRequested) error {
	verifyURL := m.buildURL("/verify-email", e.VerifyToken)
	if err := m.sender.SendVerificationEmail(ctx, e.Email, verifyURL); err != nil {
		m.log.Error("failed to send verification email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("verification email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handlePasswordResetRequested(ctx context.Context, e events.PasswordResetRequested) error {
	resetURL := m.buildURL("/reset-password", e.ResetToken)
	if err := m.sender.SendPasswordResetEmail(ctx, e.Email, resetURL); err != nil {
		m.log.Error("failed to send password reset email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("password reset email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handleOrganizationInviteCreated(ctx context.Context, e events.OrganizationInviteCreated) error {
	inviteURL := m.buildURL("/sign-up", e.InviteToken)
	if err := m.sender.SendOrganizationInviteEmail(ctx, e.Email, e.OrganizationName, inviteURL); err != nil {
		m.log.Error("failed to send organization invite email",
			"organizationId", e.OrganizationID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("organization invite email sent", "organizationId", e.OrganizationID, "email", e.Email)
	return nil
}

func (m *Module) handlePartnerInviteCreated(ctx context.Context, e events.PartnerInviteCreated) error {
	inviteURL := m.buildURL("/partner-invite", e.InviteToken)
	sender := m.resolveSender(ctx, e.OrganizationID)
	if err := sender.SendPartnerInviteEmail(ctx, e.Email, e.OrganizationName, e.PartnerName, inviteURL); err != nil {
		m.log.Error("failed to send partner invite email",
			"organizationId", e.OrganizationID,
			"partnerId", e.PartnerID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("partner invite email sent", "organizationId", e.OrganizationID, "partnerId", e.PartnerID, "email", e.Email)
	return nil
}

func (m *Module) handlePartnerOfferCreated(ctx context.Context, e events.PartnerOfferCreated) error {
	// Build the public acceptance URL for the vakman.
	acceptURL := m.buildURL("/partner-offer", e.PublicToken)

	// Build WhatsApp draft URL
	priceFormatted := fmt.Sprintf("€%.2f", float64(e.VakmanPriceCents)/100)
	whatsappMsg := fmt.Sprintf(
		partnerOfferCreatedTemplate,
		e.PartnerName, priceFormatted, acceptURL,
	)
	cleanPhone := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, e.PartnerPhone)
	whatsappURL := fmt.Sprintf("https://wa.me/%s?text=%s", cleanPhone, urlEncode(whatsappMsg))

	m.log.Info("partner offer created — acceptance URL generated",
		"offerId", e.OfferID,
		"organizationId", e.OrganizationID,
		"partnerId", e.PartnerID,
		"leadServiceId", e.LeadServiceID,
		"vakmanPriceCents", e.VakmanPriceCents,
		"acceptanceUrl", acceptURL,
	)

	// Write timeline event on the lead with WhatsApp draft
	if m.offerTimeline != nil {
		serviceID := e.LeadServiceID
		summary := fmt.Sprintf("Aanbod van %s naar %s verstuurd", priceFormatted, e.PartnerName)
		drafts := buildPartnerOfferCreatedDrafts(e.PartnerName, priceFormatted, acceptURL)
		if err := m.offerTimeline.WriteOfferEvent(ctx,
			e.LeadID, &serviceID, e.OrganizationID,
			"System", "Offer Dispatch",
			"partner_offer_created",
			"Werkaanbod verstuurd naar vakman",
			&summary,
			map[string]any{
				"offerId":          e.OfferID.String(),
				"partnerId":        e.PartnerID.String(),
				"partnerName":      e.PartnerName,
				"vakmanPriceCents": e.VakmanPriceCents,
				"publicToken":      e.PublicToken,
				"acceptanceUrl":    acceptURL,
				"whatsappUrl":      whatsappURL,
				"drafts":           drafts,
			},
		); err != nil {
			m.log.Error("failed to write partner offer timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	return nil
}

func (m *Module) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetAppBaseURL(), "/")
	return base + path + "?token=" + tokenValue
}

// ── Partner offer event handlers ────────────────────────────────────────

const partnerOfferNotificationEmail = "info@salestainable.nl"
const partnerOfferCreatedTemplate = "Hallo %s,\n\nEr is een nieuw werkaanbod voor u beschikbaar ter waarde van %s.\n\nBekijk het aanbod en geef uw beschikbaarheid door via onderstaande link:\n%s\n\nMet vriendelijke groet"
const leadWelcomeSummaryFmt = "WhatsApp welkomstbericht verstuurd naar %s"
const invalidOutboxPayloadPrefix = "invalid payload: "
const maxOutboxRetryAttempts = 5
const workflowEngineActorName = "Workflow Engine"
const quotePublicPathPrefix = "/quote/"
const noReasonProvided = "Geen reden opgegeven"
const outboxRetryBaseDelay = time.Minute
const outboxRetryMaxDelay = 60 * time.Minute

func (m *Module) handlePartnerOfferAccepted(ctx context.Context, e events.PartnerOfferAccepted) error {
	m.log.Info("partner offer accepted",
		"offerId", e.OfferID,
		"partnerId", e.PartnerID,
		"partnerName", e.PartnerName,
		"leadId", e.LeadID,
	)

	// 1. Write timeline event on the lead
	if m.offerTimeline != nil {
		serviceID := e.LeadServiceID
		summary := fmt.Sprintf("%s heeft het werkaanbod geaccepteerd en beschikbaarheid doorgegeven", e.PartnerName)
		if err := m.offerTimeline.WriteOfferEvent(ctx,
			e.LeadID, &serviceID, e.OrganizationID,
			"Partner", e.PartnerName,
			"partner_offer_accepted",
			"Werkaanbod geaccepteerd",
			&summary,
			map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
			},
		); err != nil {
			m.log.Error("failed to write partner offer accepted timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	// 2. Send notification email to info@salestainable.nl
	if err := m.sender.SendPartnerOfferAcceptedEmail(ctx, partnerOfferNotificationEmail, e.PartnerName, e.OfferID.String()); err != nil {
		m.log.Error("failed to send partner offer accepted email",
			"offerId", e.OfferID,
			"error", err,
		)
		// Non-fatal: continue to send confirmation to vakman
	}
	m.log.Info("partner offer accepted email sent",
		"offerId", e.OfferID,
		"toEmail", partnerOfferNotificationEmail,
	)

	// 3. Send confirmation email to the vakman
	if e.PartnerEmail != "" {
		sender := m.resolveSender(ctx, e.OrganizationID)
		if err := sender.SendPartnerOfferAcceptedConfirmationEmail(ctx, e.PartnerEmail, e.PartnerName); err != nil {
			m.log.Error("failed to send partner offer accepted confirmation to vakman",
				"offerId", e.OfferID,
				"partnerEmail", e.PartnerEmail,
				"error", err,
			)
		} else {
			m.log.Info("partner offer accepted confirmation sent to vakman",
				"offerId", e.OfferID,
				"partnerEmail", e.PartnerEmail,
			)
		}
	}

	if e.PartnerPhone != "" && e.PartnerWhatsAppOptedIn {
		msg := fmt.Sprintf(
			"Bedankt %s! 🔨\n\nU heeft de klus geaccepteerd (Offer ID: %s). We hebben de klant geïnformeerd.\n\nWe sturen u zo snel mogelijk de definitieve details voor de inspectie.",
			e.PartnerName,
			e.OfferID.String()[:8],
		)
		_ = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
			Ctx:         ctx,
			OrgID:       e.OrganizationID,
			LeadID:      &e.LeadID,
			ServiceID:   &e.LeadServiceID,
			PhoneNumber: e.PartnerPhone,
			Message:     msg,
			Category:    "partner_offer_accepted",
			Audience:    "partner",
			Summary:     fmt.Sprintf("WhatsApp bevestiging verstuurd naar %s", e.PartnerName),
			ActorType:   "System",
			ActorName:   "Portal",
		})
	}

	return nil
}

func (m *Module) handlePartnerOfferRejected(ctx context.Context, e events.PartnerOfferRejected) error {
	m.log.Info("partner offer rejected",
		"offerId", e.OfferID,
		"partnerId", e.PartnerID,
		"partnerName", e.PartnerName,
		"reason", e.Reason,
	)

	// 1. Write timeline event on the lead
	if m.offerTimeline != nil {
		serviceID := e.LeadServiceID
		summary := fmt.Sprintf("%s heeft het werkaanbod afgewezen", e.PartnerName)
		if e.Reason != "" {
			summary += fmt.Sprintf(" — reden: %s", e.Reason)
		}
		drafts := buildPartnerOfferRejectedDrafts(e.PartnerName, e.Reason)
		if err := m.offerTimeline.WriteOfferEvent(ctx,
			e.LeadID, &serviceID, e.OrganizationID,
			"Partner", e.PartnerName,
			"partner_offer_rejected",
			"Werkaanbod afgewezen",
			&summary,
			map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
				"reason":      e.Reason,
				"drafts":      drafts,
			},
		); err != nil {
			m.log.Error("failed to write partner offer rejected timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	// 2. Send notification email to info@salestainable.nl
	if err := m.sender.SendPartnerOfferRejectedEmail(ctx, partnerOfferNotificationEmail, e.PartnerName, e.OfferID.String(), e.Reason); err != nil {
		m.log.Error("failed to send partner offer rejected email",
			"offerId", e.OfferID,
			"error", err,
		)
		return err
	}
	m.log.Info("partner offer rejected email sent",
		"offerId", e.OfferID,
		"toEmail", partnerOfferNotificationEmail,
	)

	return nil
}

func (m *Module) handlePartnerOfferExpired(ctx context.Context, e events.PartnerOfferExpired) error {
	m.log.Info("partner offer expired",
		"offerId", e.OfferID,
		"partnerId", e.PartnerID,
		"partnerName", e.PartnerName,
	)

	// Write timeline event on the lead
	if m.offerTimeline != nil {
		serviceID := e.LeadServiceID
		summary := fmt.Sprintf("Werkaanbod naar %s is verlopen zonder reactie", e.PartnerName)
		drafts := buildPartnerOfferExpiredDrafts(e.PartnerName)
		if err := m.offerTimeline.WriteOfferEvent(ctx,
			e.LeadID, &serviceID, e.OrganizationID,
			"System", "Offer Expiry",
			"partner_offer_expired",
			"Werkaanbod verlopen",
			&summary,
			map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
				"drafts":      drafts,
			},
		); err != nil {
			m.log.Error("failed to write partner offer expired timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	return nil
}

func (m *Module) handleLeadCreated(ctx context.Context, e events.LeadCreated) error {
	m.log.Info("processing lead created notification", "leadId", e.LeadID, "orgId", e.TenantID, "source", strings.TrimSpace(e.Source), "leadServiceId", e.LeadServiceID)
	rule, shouldSend := m.resolveLeadWelcomeRule(ctx, e)
	if !shouldSend {
		return nil
	}
	if !e.WhatsAppOptedIn {
		m.log.Info("whatsapp disabled for lead, skipping welcome message", "leadId", e.LeadID)
		return nil
	}
	if strings.TrimSpace(e.ConsumerPhone) == "" {
		m.log.Info("missing consumer phone; skipping lead welcome", "leadId", e.LeadID, "orgId", e.TenantID)
		return nil
	}

	consumerName := defaultName(strings.TrimSpace(e.ConsumerName), "daar")
	message := m.buildLeadWelcomeText(ctx, e, rule, consumerName)

	_ = m.enqueueLeadWelcomeOutbox(ctx, e, rule, message, consumerName)
	return nil
}

func (m *Module) resolveLeadWelcomeRule(ctx context.Context, e events.LeadCreated) (*workflowRule, bool) {
	if strings.EqualFold(strings.TrimSpace(e.Source), "quote_flow") {
		m.log.Info("lead created from quote flow, skipping welcome message", "leadId", e.LeadID)
		return nil, false
	}

	source := strings.TrimSpace(e.Source)
	var leadSource *string
	if source != "" {
		leadSource = &source
	}
	rule := m.resolveWorkflowRule(ctx, e.TenantID, e.LeadID, "lead_welcome", "whatsapp", "lead", leadSource)
	if rule == nil {
		m.log.Info("no workflow rule for lead welcome; skipping", "leadId", e.LeadID, "orgId", e.TenantID)
		return nil, false
	}
	if !rule.Enabled {
		m.log.Info("workflow disabled: skipping lead welcome", "leadId", e.LeadID)
		return rule, false
	}

	return rule, true
}

func (m *Module) buildLeadWelcomeText(ctx context.Context, e events.LeadCreated, rule *workflowRule, consumerName string) string {
	trackLink := m.buildLeadTrackLink(e.PublicToken)
	if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
		rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
			"lead": map[string]any{
				"name":   consumerName,
				"phone":  e.ConsumerPhone,
				"source": strings.TrimSpace(e.Source),
			},
			"links": map[string]any{
				"track": trackLink,
			},
		})
		if err == nil && strings.TrimSpace(rendered) != "" {
			return rendered
		}
		if err != nil {
			m.log.Warn("failed to render workflow template; using default", "error", err, "trigger", "lead_welcome")
		}
	}

	_ = ctx
	return buildLeadWelcomeMessage(consumerName, trackLink)
}

func (m *Module) enqueueLeadWelcomeOutbox(ctx context.Context, e events.LeadCreated, rule *workflowRule, message, consumerName string) bool {
	if m.notificationOutbox == nil {
		m.log.Warn("notification outbox not configured; lead welcome not enqueued", "leadId", e.LeadID, "orgId", e.TenantID)
		return false
	}

	delayMinutes := rule.DelayMinutes
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
		OrgID:          e.TenantID,
		LeadID:         &e.LeadID,
		ServiceID:      &e.LeadServiceID,
		LeadPhone:      e.ConsumerPhone,
		Trigger:        "lead_welcome",
		DefaultSummary: fmt.Sprintf(leadWelcomeSummaryFmt, consumerName),
		DefaultActor:   "System",
		DefaultOrigin:  "Portal",
	})
	if err != nil {
		m.log.Warn("failed to enqueue whatsapp lead welcome outbox", "error", err, "leadId", e.LeadID)
		return false
	}
	m.log.Info("lead welcome queued via outbox", "leadId", e.LeadID, "orgId", e.TenantID, "delayMinutes", delayMinutes)
	return true
}

func ptrUUIDString(v *uuid.UUID) *string {
	if v == nil {
		return nil
	}
	s := v.String()
	return &s
}

func (m *Module) handleNotificationOutboxDue(ctx context.Context, e events.NotificationOutboxDue) error {
	if m.notificationOutbox == nil {
		m.log.Debug("notification outbox repository not configured; skipping outbox due event", "outboxId", e.OutboxID, "tenantId", e.TenantID)
		return nil
	}
	m.log.Info("processing outbox due event", "outboxId", e.OutboxID, "tenantId", e.TenantID)
	rec, process, err := m.prepareOutboxRecord(ctx, e.OutboxID)
	if err != nil || !process {
		if err != nil {
			m.log.Error("failed to prepare outbox record", "outboxId", e.OutboxID, "error", err)
		}
		return err
	}

	if rec.Kind != "whatsapp" && rec.Kind != "email" {
		m.markOutboxUnsupported(ctx, rec)
		return nil
	}

	var processErr error
	switch rec.Template {
	case "whatsapp_send":
		processErr = m.processGenericWhatsAppOutbox(ctx, e, rec)
	case "email_send":
		processErr = m.processGenericEmailOutbox(ctx, e, rec)
	default:
		m.markOutboxUnsupported(ctx, rec)
		return nil
	}

	if processErr != nil {
		m.handleOutboxDeliveryError(ctx, rec, processErr)
		return processErr
	}
	m.log.Info("outbox record processed successfully", "outboxId", rec.ID, "kind", rec.Kind, "template", rec.Template)

	return nil
}

func (m *Module) handleOutboxDeliveryError(ctx context.Context, rec notificationoutbox.Record, deliveryErr error) {
	attempt := rec.Attempts + 1
	if attempt >= maxOutboxRetryAttempts {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, deliveryErr.Error())
		m.log.Warn("notification outbox exhausted retries",
			"outboxId", rec.ID,
			"kind", rec.Kind,
			"template", rec.Template,
			"attempt", attempt,
			"maxAttempts", maxOutboxRetryAttempts,
			"error", deliveryErr,
		)
		return
	}

	retryAt := time.Now().UTC().Add(computeOutboxRetryDelay(attempt))
	if err := m.notificationOutbox.ScheduleRetry(ctx, rec.ID, retryAt, deliveryErr.Error()); err != nil {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, deliveryErr.Error())
		m.log.Error("notification outbox retry scheduling failed; marked failed",
			"outboxId", rec.ID,
			"attempt", attempt,
			"error", err,
		)
		return
	}

	m.log.Warn("notification outbox scheduled retry",
		"outboxId", rec.ID,
		"kind", rec.Kind,
		"template", rec.Template,
		"attempt", attempt,
		"maxAttempts", maxOutboxRetryAttempts,
		"retryAt", retryAt,
		"error", deliveryErr,
	)
}

func computeOutboxRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := outboxRetryBaseDelay << (attempt - 1)
	if delay > outboxRetryMaxDelay {
		return outboxRetryMaxDelay
	}
	return delay
}

func (m *Module) prepareOutboxRecord(ctx context.Context, outboxID uuid.UUID) (notificationoutbox.Record, bool, error) {
	rec, err := m.notificationOutbox.GetByID(ctx, outboxID)
	if err != nil {
		return notificationoutbox.Record{}, false, err
	}
	if rec.Status == notificationoutbox.StatusSucceeded {
		m.log.Debug("outbox record already succeeded; skipping", "outboxId", rec.ID)
		return rec, false, nil
	}
	if err := m.notificationOutbox.MarkProcessing(ctx, rec.ID); err != nil {
		return notificationoutbox.Record{}, false, err
	}
	m.log.Debug("outbox record marked processing", "outboxId", rec.ID, "kind", rec.Kind, "template", rec.Template)
	return rec, true, nil
}

func (m *Module) processGenericWhatsAppOutbox(ctx context.Context, e events.NotificationOutboxDue, rec notificationoutbox.Record) error {
	var payload whatsAppSendOutboxPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, invalidOutboxPayloadPrefix+err.Error())
		return nil
	}
	if strings.TrimSpace(payload.PhoneNumber) == "" {
		m.log.Debug("outbox whatsapp payload has no phone number; marking succeeded", "outboxId", rec.ID)
		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	orgID := e.TenantID
	if strings.TrimSpace(payload.OrgID) != "" {
		if parsed, err := uuid.Parse(payload.OrgID); err == nil {
			orgID = parsed
		}
	}

	leadID := parseOptionalUUID(payload.LeadID)
	svcID := parseOptionalUUID(payload.ServiceID)
	if leadID != nil && !m.isLeadWhatsAppOptedIn(ctx, *leadID, orgID) {
		m.log.Info("lead opted out; skipping whatsapp outbox send", "outboxId", rec.ID, "leadId", *leadID, "orgId", orgID)
		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	err := m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
		Ctx:         ctx,
		OrgID:       orgID,
		LeadID:      leadID,
		ServiceID:   svcID,
		PhoneNumber: payload.PhoneNumber,
		Message:     payload.Message,
		Category:    payload.Category,
		Audience:    payload.Audience,
		Summary:     payload.Summary,
		ActorType:   payload.ActorType,
		ActorName:   payload.ActorName,
		Metadata:    payload.Metadata,
	})
	if err != nil {
		return err
	}

	_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
	m.log.Info("whatsapp outbox delivered", "outboxId", rec.ID, "orgId", orgID, "phone", payload.PhoneNumber, "category", payload.Category)
	return nil
}

func (m *Module) processGenericEmailOutbox(ctx context.Context, e events.NotificationOutboxDue, rec notificationoutbox.Record) error {
	var payload emailSendOutboxPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, invalidOutboxPayloadPrefix+err.Error())
		return nil
	}

	if strings.TrimSpace(payload.ToEmail) == "" {
		m.log.Debug("outbox email payload has no recipient; marking succeeded", "outboxId", rec.ID)
		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	if strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.BodyHTML) == "" {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, "invalid payload: subject and bodyHtml are required")
		return nil
	}

	orgID := e.TenantID
	if strings.TrimSpace(payload.OrgID) != "" {
		if parsed, err := uuid.Parse(payload.OrgID); err == nil {
			orgID = parsed
		}
	}

	sender := m.resolveSender(ctx, orgID)
	if err := sender.SendCustomEmail(ctx, payload.ToEmail, payload.Subject, payload.BodyHTML); err != nil {
		return err
	}

	_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
	m.log.Info("email outbox delivered", "outboxId", rec.ID, "orgId", orgID, "toEmail", payload.ToEmail)
	return nil
}

func (m *Module) markOutboxUnsupported(ctx context.Context, rec notificationoutbox.Record) {
	msg := fmt.Sprintf("unsupported outbox kind/template: %s/%s", rec.Kind, rec.Template)
	_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, msg)
	m.log.Warn("unsupported outbox record", "outboxId", rec.ID, "kind", rec.Kind, "template", rec.Template)
}

func parseOptionalUUID(value *string) *uuid.UUID {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return nil
	}
	return &parsed
}

func (m *Module) buildLeadTrackLink(publicToken string) string {
	if strings.TrimSpace(publicToken) == "" {
		return ""
	}
	base := strings.TrimRight(m.cfg.GetAppBaseURL(), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/track/%s", base, publicToken)
}

func buildLeadWelcomeMessage(consumerName string, trackLink string) string {
	if strings.TrimSpace(trackLink) == "" {
		return fmt.Sprintf(
			"Beste %s,\n\n"+
				"Bedankt voor je aanvraag! 👍\n\n"+
				"We hebben alles ontvangen en gaan het nu rustig doornemen. "+
				"Vandaag nemen we contact met je op om het verder te bespreken.",
			consumerName,
		)
	}

	return fmt.Sprintf(
		"Beste %s,\n\n"+
			"Bedankt voor je aanvraag! 👍\n\n"+
			"Volg de status of voeg details toe via jouw persoonlijke pagina:\n%s\n\n"+
			"We nemen vandaag contact met je op.",
		consumerName,
		trackLink,
	)
}

func (m *Module) handleLeadDataChanged(_ context.Context, e events.LeadDataChanged) error {
	if m.sse == nil {
		return nil
	}

	var eventType sse.EventType
	var message string

	switch e.Source {
	case "customer_preferences":
		eventType = sse.EventLeadPreferencesUpdated
		message = "Klant heeft voorkeuren bijgewerkt"
	case "customer_portal_update":
		eventType = sse.EventLeadInfoAdded
		message = "Klant heeft extra info toegevoegd"
	case "customer_portal_upload":
		eventType = sse.EventLeadAttachmentUploaded
		message = "Klant heeft bestanden geupload"
	case "customer_portal_delete":
		eventType = sse.EventLeadAttachmentDeleted
		message = "Klant heeft een bestand verwijderd"
	case "appointment_request":
		eventType = sse.EventLeadAppointmentRequested
		message = "Klant heeft een inspectie aangevraagd"
	default:
		return nil
	}

	m.sse.PublishToOrganization(e.TenantID, sse.Event{
		Type:      eventType,
		LeadID:    e.LeadID,
		ServiceID: e.LeadServiceID,
		Message:   message,
		Data: map[string]interface{}{
			"source": e.Source,
		},
	})

	return nil
}

func (m *Module) handlePipelineStageChanged(_ context.Context, e events.PipelineStageChanged) error {
	if m.sse == nil {
		return nil
	}

	m.sse.PublishToLead(e.LeadID, sse.Event{
		Type:      sse.EventLeadStatusChanged,
		LeadID:    e.LeadID,
		ServiceID: e.LeadServiceID,
		Data: map[string]interface{}{
			"oldStage": e.OldStage,
			"newStage": e.NewStage,
		},
	})

	return nil
}

// ── Quote event handlers ────────────────────────────────────────────────

func (m *Module) handleQuoteSent(ctx context.Context, e events.QuoteSent) error {
	m.publishQuoteSentEvents(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_sent",
		"Offerte verstuurd naar "+e.ConsumerName,
		map[string]interface{}{"quoteNumber": e.QuoteNumber, "consumerEmail": e.ConsumerEmail})
	m.sendQuoteSentWhatsApp(ctx, e)

	m.log.Info("quote sent event processed", "quoteId", e.QuoteID)
	return nil
}

type dispatchQuoteEmailWorkflowParams struct {
	Rule         *workflowRule
	OrgID        uuid.UUID
	LeadID       *uuid.UUID
	ServiceID    *uuid.UUID
	LeadEmail    string
	PartnerEmail string
	Trigger      string
	Subject      string
	BodyText     string
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteEmailWorkflow(ctx context.Context, p dispatchQuoteEmailWorkflowParams) bool {
	if p.Rule == nil {
		return false
	}
	if !p.Rule.Enabled {
		return true
	}
	if strings.TrimSpace(p.LeadEmail) == "" && strings.TrimSpace(p.PartnerEmail) == "" {
		return true
	}
	if m.notificationOutbox == nil {
		return false
	}

	bodyText := p.BodyText
	if p.Rule.TemplateText != nil && strings.TrimSpace(*p.Rule.TemplateText) != "" {
		bodyText = *p.Rule.TemplateText
	}
	subject := strings.TrimSpace(p.Subject)
	if subject == "" || strings.TrimSpace(bodyText) == "" {
		return true
	}
	bodyHTML := strings.ReplaceAll(bodyText, "\n", "<br/>")
	recipientConfig := map[string]any{
		"includeLeadContact": strings.TrimSpace(p.LeadEmail) != "",
		"includePartner":     strings.TrimSpace(p.PartnerEmail) != "",
	}
	steps := []repository.WorkflowStep{{
		Enabled:         true,
		Channel:         "email",
		Audience:        "lead",
		DelayMinutes:    p.Rule.DelayMinutes,
		TemplateSubject: &subject,
		TemplateBody:    &bodyHTML,
		RecipientConfig: recipientConfig,
	}}

	err := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadEmail:      p.LeadEmail,
		PartnerEmail:   p.PartnerEmail,
		Trigger:        p.Trigger,
		DefaultSummary: p.Summary,
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
	})
	if err != nil {
		m.log.Warn(p.FallbackNote, "error", err, "orgId", p.OrgID)
		return false
	}

	return true
}

type dispatchQuoteWhatsAppWorkflowParams struct {
	Rule         *workflowRule
	OrgID        uuid.UUID
	LeadID       *uuid.UUID
	ServiceID    *uuid.UUID
	LeadPhone    string
	Trigger      string
	BodyText     string
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteWhatsAppWorkflow(ctx context.Context, p dispatchQuoteWhatsAppWorkflowParams) bool {
	if p.Rule == nil {
		return false
	}
	if !p.Rule.Enabled {
		return true
	}
	if strings.TrimSpace(p.LeadPhone) == "" {
		return true
	}
	if p.LeadID != nil && !m.isLeadWhatsAppOptedIn(ctx, *p.LeadID, p.OrgID) {
		return true
	}
	if m.notificationOutbox == nil {
		return false
	}

	messageText := p.BodyText
	if p.Rule.TemplateText != nil && strings.TrimSpace(*p.Rule.TemplateText) != "" {
		messageText = *p.Rule.TemplateText
	}
	if strings.TrimSpace(messageText) == "" {
		return true
	}

	steps := []repository.WorkflowStep{{
		Enabled:      true,
		Channel:      "whatsapp",
		Audience:     "lead",
		DelayMinutes: p.Rule.DelayMinutes,
		TemplateBody: &messageText,
		RecipientConfig: map[string]any{
			"includeLeadContact": true,
		},
	}}
	err := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadPhone:      p.LeadPhone,
		Trigger:        p.Trigger,
		DefaultSummary: p.Summary,
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
	})
	if err != nil {
		m.log.Warn(p.FallbackNote, "error", err, "orgId", p.OrgID)
		return false
	}

	return true
}

func (m *Module) publishQuoteSentEvents(e events.QuoteSent) {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteSent, e.QuoteID, map[string]interface{}{
		"quoteNumber": e.QuoteNumber,
		"status":      "Sent",
	})
	if m.sse == nil {
		return
	}

	evt := sse.Event{
		Type:   sse.EventQuoteSent,
		LeadID: e.LeadID,
		Data: map[string]interface{}{
			"quoteId":     e.QuoteID,
			"quoteNumber": e.QuoteNumber,
			"status":      "Sent",
		},
	}
	if e.LeadServiceID != nil {
		evt.ServiceID = *e.LeadServiceID
	}
	m.sse.PublishToLead(e.LeadID, evt)
}

func (m *Module) sendQuoteSentWhatsApp(ctx context.Context, e events.QuoteSent) {
	if strings.TrimSpace(e.ConsumerPhone) == "" {
		return
	}
	if !m.isLeadWhatsAppOptedIn(ctx, e.LeadID, e.OrganizationID) {
		return
	}

	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_sent", "whatsapp", "lead", nil)
	if rule != nil && !rule.Enabled {
		return
	}

	proposalURL := strings.TrimRight(m.cfg.GetAppBaseURL(), "/") + quotePublicPathPrefix + e.PublicToken
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	message := m.buildQuoteSentMessage(rule, e, proposalURL, name)
	_ = m.enqueueQuoteSentOutbox(ctx, e, rule, message, name)
}

func (m *Module) buildQuoteSentMessage(rule *workflowRule, e events.QuoteSent, proposalURL, name string) string {
	if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
		rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
			"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone},
			"quote": map[string]any{"number": e.QuoteNumber, "previewUrl": proposalURL},
			"org":   map[string]any{"name": e.OrganizationName},
		})
		if err == nil && strings.TrimSpace(rendered) != "" {
			return rendered
		}
	}

	return fmt.Sprintf(
		"Hi %s,\n\nUw offerte %s van %s is klaar! 📄\n\nBekijk en accordeer hem direct via deze link:\n%s\n\nMet vriendelijke groet,\n%s",
		name,
		e.QuoteNumber,
		e.OrganizationName,
		proposalURL,
		e.OrganizationName,
	)
}

func (m *Module) enqueueQuoteSentOutbox(ctx context.Context, e events.QuoteSent, rule *workflowRule, message, name string) bool {
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
		OrgID:          e.OrganizationID,
		LeadID:         &e.LeadID,
		ServiceID:      e.LeadServiceID,
		LeadPhone:      e.ConsumerPhone,
		Trigger:        "quote_sent",
		DefaultSummary: fmt.Sprintf("WhatsApp offerte verstuurd naar %s", name),
		DefaultActor:   "System",
		DefaultOrigin:  "Portal",
	})
	return err == nil
}

func (m *Module) handleAppointmentCreated(ctx context.Context, e events.AppointmentCreated) error {
	return m.handleAppointmentWhatsApp(ctx, appointmentWhatsAppParams{
		OrgID:         e.OrganizationID,
		LeadID:        e.LeadID,
		ServiceID:     e.LeadServiceID,
		Type:          e.Type,
		ConsumerPhone: e.ConsumerPhone,
		ConsumerName:  e.ConsumerName,
		StartTime:     e.StartTime,
		Location:      e.Location,
		Trigger:       "appointment_created",
		Category:      "appointment_created",
		SummaryFmt:    "WhatsApp afspraakbevestiging verstuurd naar %s",
		DefaultMessage: func(name, dateStr, timeStr string) string {
			return fmt.Sprintf(
				"Hi %s,\n\nUw afspraak is bevestigd! ✅\n\nDatum: %s\nTijd: %s\n\nOnze adviseur komt bij u langs voor de opname. Tot dan!",
				name,
				dateStr,
				timeStr,
			)
		},
	})
}

func (m *Module) handleAppointmentReminderDue(ctx context.Context, e events.AppointmentReminderDue) error {
	return m.handleAppointmentWhatsApp(ctx, appointmentWhatsAppParams{
		OrgID:         e.OrganizationID,
		LeadID:        e.LeadID,
		ServiceID:     e.LeadServiceID,
		Type:          e.Type,
		ConsumerPhone: e.ConsumerPhone,
		ConsumerName:  e.ConsumerName,
		StartTime:     e.StartTime,
		Location:      e.Location,
		Trigger:       "appointment_reminder",
		Category:      "appointment_reminder",
		SummaryFmt:    "WhatsApp afspraakherinnering verstuurd naar %s",
		DefaultMessage: func(name, dateStr, timeStr string) string {
			return fmt.Sprintf(
				"Herinnering, %s! ⏰\n\nMorgen staat uw afspraak gepland.\n\nDatum: %s\nTijd: %s\n\nTot morgen!",
				name,
				dateStr,
				timeStr,
			)
		},
	})
}

type appointmentWhatsAppParams struct {
	OrgID          uuid.UUID
	LeadID         *uuid.UUID
	ServiceID      *uuid.UUID
	Type           string
	ConsumerPhone  string
	ConsumerName   string
	StartTime      time.Time
	Location       string
	Trigger        string
	Category       string
	SummaryFmt     string
	DefaultMessage func(name, dateStr, timeStr string) string
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
	dateStr := p.StartTime.Format("02-01-2006")
	timeStr := p.StartTime.Format("15:04")
	message := buildAppointmentMessage(rule, name, p.ConsumerPhone, dateStr, timeStr, p.Location, p.DefaultMessage)
	_ = m.enqueueAppointmentOutbox(ctx, p, rule, message, name)
	return nil
}

func buildAppointmentMessage(rule *workflowRule, name, phone, dateStr, timeStr, location string, fallback func(name, dateStr, timeStr string) string) string {
	if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
		rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
			"lead":        map[string]any{"name": name, "phone": phone},
			"appointment": map[string]any{"date": dateStr, "time": timeStr, "location": strings.TrimSpace(location)},
		})
		if err == nil && strings.TrimSpace(rendered) != "" {
			return rendered
		}
	}

	return fallback(name, dateStr, timeStr)
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

func defaultName(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (m *Module) handleQuoteViewed(ctx context.Context, e events.QuoteViewed) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteViewed, e.QuoteID, map[string]interface{}{
		"viewerIp": e.ViewerIP,
	})
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_viewed",
		"Klant heeft de offerte geopend",
		map[string]interface{}{"viewerIp": e.ViewerIP})
	m.log.Info("quote viewed event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) handleQuoteUpdatedByCustomer(ctx context.Context, e events.QuoteUpdatedByCustomer) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteItemToggled, e.QuoteID, map[string]interface{}{
		"itemId":          e.ItemID,
		"itemDescription": e.ItemDescription,
		"isSelected":      e.IsSelected,
		"newTotalCents":   e.NewTotalCents,
	})
	action := "uitgeschakeld"
	if e.IsSelected {
		action = "ingeschakeld"
	}
	desc := e.ItemDescription
	if desc == "" {
		desc = "een item"
	}
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_item_toggled",
		"Klant heeft '"+truncate(desc, 60)+"' "+action,
		map[string]interface{}{"itemId": e.ItemID.String(), "itemDescription": e.ItemDescription, "isSelected": e.IsSelected, "newTotalCents": e.NewTotalCents})
	m.log.Info("quote item toggled event processed", "quoteId", e.QuoteID, "itemId", e.ItemID)
	return nil
}

func (m *Module) handleQuoteAnnotated(ctx context.Context, e events.QuoteAnnotated) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteAnnotated, e.QuoteID, map[string]interface{}{
		"itemId":     e.ItemID,
		"authorType": e.AuthorType,
		"text":       e.Text,
	})
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_annotated",
		"Nieuwe vraag: \""+truncate(e.Text, 80)+"\"",
		map[string]interface{}{"itemId": e.ItemID.String(), "authorType": e.AuthorType, "text": e.Text})
	m.log.Info("quote annotated event processed", "quoteId", e.QuoteID, "itemId", e.ItemID)
	return nil
}

func (m *Module) handleQuoteAccepted(ctx context.Context, e events.QuoteAccepted) error {
	_ = m.dispatchQuoteAcceptedLeadEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteAcceptedAgentEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteAcceptedLeadWhatsAppWorkflow(ctx, e)
	m.publishQuoteAcceptedSSE(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_accepted",
		"Offerte geaccepteerd door "+e.SignatureName,
		map[string]interface{}{"signatureName": e.SignatureName, "totalCents": e.TotalCents, "consumerName": e.ConsumerName})

	m.log.Info("quote accepted event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) dispatchQuoteAcceptedLeadEmailWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	subject := fmt.Sprintf("Bevestiging: offerte %s geaccepteerd", e.QuoteNumber)
	bodyText := fmt.Sprintf("Hallo %s,\n\nBedankt voor het accepteren van offerte %s. Wij verwerken uw akkoord en nemen snel contact met u op.\n\nMet vriendelijke groet,\n%s", name, e.QuoteNumber, e.OrganizationName)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_accepted",
		Subject:      subject,
		BodyText:     bodyText,
		Summary:      fmt.Sprintf("Email bevestiging offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted lead email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedAgentEmailWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.AgentName), "adviseur")
	subject := fmt.Sprintf("Offerte %s is geaccepteerd", e.QuoteNumber)
	bodyText := fmt.Sprintf("Hallo %s,\n\nOfferte %s is geaccepteerd door %s.\nTotaal: €%.2f\n\nGroet,\nWorkflow Engine", name, e.QuoteNumber, defaultName(strings.TrimSpace(e.ConsumerName), "de klant"), float64(e.TotalCents)/100)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "partner", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		PartnerEmail: e.AgentEmail,
		Trigger:      "quote_accepted",
		Subject:      subject,
		BodyText:     bodyText,
		Summary:      fmt.Sprintf("Email offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted partner email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	bodyText := fmt.Sprintf("Hallo %s, bedankt voor het accepteren van offerte %s. Wij nemen snel contact met u op voor de volgende stappen.\n\nMet vriendelijke groet, %s", name, e.QuoteNumber, e.OrganizationName)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_accepted",
		BodyText:     bodyText,
		Summary:      fmt.Sprintf("WhatsApp offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted lead whatsapp workflow",
	})
}

func (m *Module) publishQuoteAcceptedSSE(e events.QuoteAccepted) {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteAccepted, e.QuoteID, map[string]interface{}{
		"signatureName": e.SignatureName,
		"totalCents":    e.TotalCents,
	})

	if m.sse == nil {
		return
	}

	evt := sse.Event{
		Type:   sse.EventQuoteAccepted,
		LeadID: e.LeadID,
		Data: map[string]interface{}{
			"quoteId":   e.QuoteID,
			"status":    "Accepted",
			"signature": e.SignatureName,
		},
	}
	if e.LeadServiceID != nil {
		evt.ServiceID = *e.LeadServiceID
	}
	m.sse.PublishToLead(e.LeadID, evt)
}

func (m *Module) handleQuoteRejected(ctx context.Context, e events.QuoteRejected) error {
	_ = m.dispatchQuoteRejectedLeadEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteRejectedLeadWhatsAppWorkflow(ctx, e)

	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteRejected, e.QuoteID, map[string]interface{}{
		"reason": e.Reason,
	})

	if m.sse != nil {
		evt := sse.Event{
			Type:   sse.EventQuoteRejected,
			LeadID: e.LeadID,
			Data: map[string]interface{}{
				"quoteId": e.QuoteID,
				"status":  "Rejected",
				"reason":  e.Reason,
			},
		}
		if e.LeadServiceID != nil {
			evt.ServiceID = *e.LeadServiceID
		}
		m.sse.PublishToLead(e.LeadID, evt)
	}
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_rejected",
		"Offerte afgewezen door klant",
		map[string]interface{}{"reason": e.Reason})
	m.log.Info("quote rejected event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) dispatchQuoteRejectedLeadEmailWorkflow(ctx context.Context, e events.QuoteRejected) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	reason := defaultName(strings.TrimSpace(e.Reason), noReasonProvided)
	subject := "We hebben uw beslissing ontvangen - offerte"
	bodyText := fmt.Sprintf("Hallo %s,\n\nWij hebben uw beslissing over de offerte ontvangen. Reden: %s.\n\nAls u vragen heeft of wilt overleggen, helpen wij graag.\n\nMet vriendelijke groet,\n%s", name, reason, defaultName(strings.TrimSpace(e.OrganizationName), "ons team"))
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_rejected",
		Subject:      subject,
		BodyText:     bodyText,
		Summary:      fmt.Sprintf("Email offerteafwijzing bevestigd naar %s", name),
		FallbackNote: "failed to enqueue quote_rejected lead email workflow",
	})
}

func (m *Module) dispatchQuoteRejectedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteRejected) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	reason := defaultName(strings.TrimSpace(e.Reason), noReasonProvided)
	bodyText := fmt.Sprintf("Hallo %s, wij hebben uw beslissing over de offerte ontvangen. Reden: %s. Als u vragen heeft, helpen wij graag.\n\nMet vriendelijke groet, %s", name, reason, defaultName(strings.TrimSpace(e.OrganizationName), "ons team"))
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_rejected",
		BodyText:     bodyText,
		Summary:      fmt.Sprintf("WhatsApp offerteafwijzing bevestigd naar %s", name),
		FallbackNote: "failed to enqueue quote_rejected lead whatsapp workflow",
	})
}

// pushQuoteSSE broadcasts a quote event to all connected agents in the org via SSE.
func (m *Module) pushQuoteSSE(orgID uuid.UUID, eventType sse.EventType, quoteID uuid.UUID, data interface{}) {
	if m.sse == nil {
		return
	}
	m.sse.PublishQuoteEvent(orgID, eventType, quoteID, data)
}

// logQuoteActivity persists an activity record for a quote. Failures are logged but non-fatal.
func (m *Module) logQuoteActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) {
	if m.actWriter == nil {
		return
	}
	if err := m.actWriter.CreateActivity(ctx, quoteID, orgID, eventType, message, metadata); err != nil {
		m.log.Error("failed to persist quote activity",
			"quoteId", quoteID,
			"eventType", eventType,
			"error", err,
		)
	}
}

// truncate shortens a string to max characters, appending "…" when truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// urlEncode percent-encodes a string for use in a URL query parameter.
func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func buildPartnerOfferCreatedDrafts(partnerName, priceFormatted, acceptURL string) map[string]any {
	emailSubject := "Nieuw werkaanbod beschikbaar"
	emailBody := fmt.Sprintf(partnerOfferCreatedTemplate, partnerName, priceFormatted, acceptURL)
	whatsAppMessage := fmt.Sprintf(partnerOfferCreatedTemplate, partnerName, priceFormatted, acceptURL)

	return map[string]any{
		"emailSubject":    emailSubject,
		"emailBody":       emailBody,
		"whatsappMessage": whatsAppMessage,
		"messageLanguage": "nl",
		"messageAudience": "partner",
		"messageCategory": "partner_offer_created",
		"status":          "draft",
	}
}

func buildPartnerOfferRejectedDrafts(partnerName, reason string) map[string]any {
	cleanReason := strings.TrimSpace(reason)
	if cleanReason == "" {
		cleanReason = "Geen reden opgegeven"
	}

	emailSubject := "Werkaanbod afgewezen"
	emailBody := fmt.Sprintf("Hallo %s,\n\nWij hebben uw afwijzing ontvangen. Reden: %s.\n\nAls u toch beschikbaar bent of aanvullende vragen heeft, laat het ons weten.\n\nMet vriendelijke groet", partnerName, cleanReason)
	whatsAppMessage := fmt.Sprintf("Hallo %s,\n\nWij hebben uw afwijzing ontvangen. Reden: %s.\n\nAls u toch beschikbaar bent of aanvullende vragen heeft, laat het ons weten.", partnerName, cleanReason)

	return map[string]any{
		"emailSubject":    emailSubject,
		"emailBody":       emailBody,
		"whatsappMessage": whatsAppMessage,
		"messageLanguage": "nl",
		"messageAudience": "partner",
		"messageCategory": "partner_offer_rejected",
		"status":          "draft",
	}
}

func buildPartnerOfferExpiredDrafts(partnerName string) map[string]any {
	emailSubject := "Werkaanbod verlopen"
	emailBody := fmt.Sprintf("Hallo %s,\n\nHet werkaanbod is verlopen zonder reactie. Als u alsnog beschikbaar bent, laat het ons weten.\n\nMet vriendelijke groet", partnerName)
	whatsAppMessage := fmt.Sprintf("Hallo %s,\n\nHet werkaanbod is verlopen zonder reactie. Als u alsnog beschikbaar bent, laat het ons weten.", partnerName)

	return map[string]any{
		"emailSubject":    emailSubject,
		"emailBody":       emailBody,
		"whatsappMessage": whatsAppMessage,
		"messageLanguage": "nl",
		"messageAudience": "partner",
		"messageCategory": "partner_offer_expired",
		"status":          "draft",
	}
}

type whatsAppBestEffortParams struct {
	Ctx         context.Context
	OrgID       uuid.UUID
	LeadID      *uuid.UUID
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

func (m *Module) sendWhatsAppBestEffort(params whatsAppBestEffortParams) error {
	if m.whatsapp == nil || params.PhoneNumber == "" {
		return nil
	}

	deviceID := m.resolveWhatsAppDeviceID(params.Ctx, params.OrgID)
	err := m.whatsapp.SendMessage(params.Ctx, deviceID, params.PhoneNumber, params.Message)
	if err != nil {
		if errors.Is(err, whatsapp.ErrNoDevice) {
			m.log.Debug("whatsapp skipped: no device configured", "orgId", params.OrgID)
			return nil
		}

		m.log.Warn("failed to send whatsapp", "error", err, "orgId", params.OrgID)
		if params.LeadID != nil {
			m.writeWhatsAppFailureEvent(params.Ctx, *params.LeadID, params.ServiceID, params.OrgID, err.Error())
		}
		return err
	}

	if params.LeadID == nil {
		return nil
	}

	metadata := params.Metadata
	if metadata == nil {
		metadata = buildWhatsAppSentMetadata(params.Category, params.Audience, params.PhoneNumber, params.Message)
	}

	m.writeWhatsAppSentEventWithMetadata(whatsAppSentEventWithMetadataParams{
		Ctx:       params.Ctx,
		LeadID:    *params.LeadID,
		ServiceID: params.ServiceID,
		OrgID:     params.OrgID,
		ActorType: params.ActorType,
		ActorName: params.ActorName,
		Summary:   params.Summary,
		Metadata:  metadata,
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

func (m *Module) resolveWhatsAppDeviceID(ctx context.Context, orgID uuid.UUID) string {
	if m.settingsReader == nil {
		return ""
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to fetch org settings for whatsapp", "error", err, "orgId", orgID)
		return ""
	}
	if settings.WhatsAppDeviceID == nil {
		return ""
	}
	return *settings.WhatsAppDeviceID
}

func (m *Module) writeWhatsAppFailureEvent(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, orgID uuid.UUID, errorMsg string) {
	if m.leadTimeline == nil {
		return
	}

	friendlyError := "Verzenden mislukt"
	msgLower := strings.ToLower(errorMsg)
	if strings.Contains(msgLower, "disconnected") || strings.Contains(msgLower, "not connected") {
		friendlyError = "Telefoon niet verbonden"
	}

	summary := fmt.Sprintf("WhatsApp niet verstuurd: %s", friendlyError)
	_ = m.leadTimeline.CreateTimelineEvent(ctx, LeadTimelineEventParams{
		LeadID:    leadID,
		ServiceID: serviceID,
		OrgID:     orgID,
		ActorType: "System",
		ActorName: "WhatsApp",
		EventType: "whatsapp_failed",
		Title:     "WhatsApp fout",
		Summary:   &summary,
		Metadata: map[string]any{
			"raw_error": errorMsg,
		},
	})
}

func buildWhatsAppSentMetadata(category, audience, phoneNumber, message string) map[string]any {
	return map[string]any{
		"status":          "sent",
		"messageCategory": category,
		"messageAudience": audience,
		"messageLanguage": "nl",
		"phoneNumber":     phone.NormalizeE164(phoneNumber),
		"messageContent":  message,
		"sentAt":          time.Now().UTC().Format(time.RFC3339),
	}
}

type whatsAppSentEventWithMetadataParams struct {
	Ctx       context.Context
	LeadID    uuid.UUID
	ServiceID *uuid.UUID
	OrgID     uuid.UUID
	ActorType string
	ActorName string
	Summary   string
	Metadata  map[string]any
}

func (m *Module) writeWhatsAppSentEventWithMetadata(params whatsAppSentEventWithMetadataParams) {
	if m.leadTimeline == nil {
		return
	}

	if err := m.leadTimeline.CreateTimelineEvent(params.Ctx, LeadTimelineEventParams{
		LeadID:    params.LeadID,
		ServiceID: params.ServiceID,
		OrgID:     params.OrgID,
		ActorType: params.ActorType,
		ActorName: params.ActorName,
		EventType: "whatsapp_sent",
		Title:     "WhatsApp verstuurd",
		Summary:   &params.Summary,
		Metadata:  params.Metadata,
	}); err != nil {
		m.log.Error("failed to write whatsapp timeline event", "error", err, "leadId", params.LeadID)
	}
}
