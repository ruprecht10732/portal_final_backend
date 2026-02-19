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
	"html"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/identity/smtpcrypto"
	leadrepo "portal_final_backend/internal/leads/repository"
	notifhandler "portal_final_backend/internal/notification/handler"
	"portal_final_backend/internal/notification/inapp"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuotePDFGenerator generates and stores an unsigned PDF for a quote.
type QuotePDFGenerator interface {
	RegeneratePDF(ctx context.Context, quoteID, organizationID uuid.UUID) (string, []byte, error)
}

// QuoteActivityWriter persists activity log entries for quotes.
type QuoteActivityWriter interface {
	CreateActivity(ctx context.Context, quoteID, orgID uuid.UUID, eventType, message string, metadata map[string]interface{}) error
}

// PartnerOfferTimelineEventParams describes the payload for a partner-offer timeline event.
// Kept as a struct to avoid long parameter lists at call sites.
type PartnerOfferTimelineEventParams struct {
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

// PartnerOfferTimelineWriter writes partner-offer events into the leads timeline.
type PartnerOfferTimelineWriter interface {
	WriteOfferEvent(ctx context.Context, params PartnerOfferTimelineEventParams) error
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

// OrganizationMemberReader lists organization users for fan-out notifications.
type OrganizationMemberReader interface {
	ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]leadrepo.OrgMember, error)
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

type cachedOrgName struct {
	name      string
	expiresAt time.Time
}

// Module handles all notification-related event subscriptions.
type Module struct {
	pool               *pgxpool.Pool
	sender             email.Sender
	cfg                config.NotificationConfig
	log                *logger.Logger
	sse                *sse.Service
	actWriter          QuoteActivityWriter
	offerTimeline      PartnerOfferTimelineWriter
	quotePDFGen        QuotePDFGenerator
	whatsapp           WhatsAppSender
	leadTimeline       LeadTimelineWriter
	settingsReader     OrganizationSettingsReader
	workflowResolver   WorkflowResolver
	leadWhatsAppReader LeadWhatsAppReader
	orgMemberReader    OrganizationMemberReader
	notificationOutbox *notificationoutbox.Repository
	inAppService       *inapp.Service
	inAppHandler       *notifhandler.HTTPHandler
	smtpEncryptionKey  []byte
	senderCache        sync.Map // map[uuid.UUID]cachedSender
	orgNameCache       sync.Map // map[uuid.UUID]cachedOrgName
}

// New creates a new notification module.
func New(pool *pgxpool.Pool, sender email.Sender, cfg config.NotificationConfig, log *logger.Logger) *Module {
	inAppRepo := inapp.NewRepository(pool)
	inAppSvc := inapp.NewService(inAppRepo, log)

	return &Module{
		pool:         pool,
		sender:       sender,
		cfg:          cfg,
		log:          log,
		inAppService: inAppSvc,
		inAppHandler: notifhandler.NewHTTPHandler(inAppSvc),
	}
}

func (m *Module) resolveOrganizationName(ctx context.Context, orgID uuid.UUID) string {
	if orgID == uuid.Nil {
		return ""
	}
	if cached, ok := m.orgNameCache.Load(orgID); ok {
		entry := cached.(cachedOrgName)
		if time.Now().Before(entry.expiresAt) {
			return entry.name
		}
		m.orgNameCache.Delete(orgID)
	}
	if m.pool == nil {
		return ""
	}
	var name string
	if err := m.pool.QueryRow(ctx, `SELECT name FROM rac_organizations WHERE id = $1`, orgID).Scan(&name); err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	m.orgNameCache.Store(orgID, cachedOrgName{name: name, expiresAt: time.Now().Add(10 * time.Minute)})
	return name
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
}

// resolveLeadDetails fetches first/last name, address and service type for a lead.
func (m *Module) resolveLeadDetails(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) *leadDetails {
	if m.pool == nil || leadID == uuid.Nil {
		return nil
	}
	var d leadDetails
	err := m.pool.QueryRow(ctx,
		`SELECT consumer_first_name, consumer_last_name, consumer_phone, consumer_email,
		        address_street, address_house_number, address_zip_code, address_city, service_type
		   FROM rac_leads WHERE id = $1 AND organization_id = $2`,
		leadID, orgID,
	).Scan(&d.FirstName, &d.LastName, &d.Phone, &d.Email,
		&d.Street, &d.HouseNumber, &d.ZipCode, &d.City, &d.ServiceType)
	if err != nil {
		return nil
	}
	return &d
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

// Name returns the module identifier.
func (m *Module) Name() string { return "notification" }

// RegisterRoutes registers notification API routes.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	if m.inAppHandler == nil {
		return
	}

	notifications := ctx.Protected.Group("/notifications")
	m.inAppHandler.RegisterRoutes(notifications)
}

// SetSSE injects the SSE service so quote events can be pushed to agents.
func (m *Module) SetSSE(s *sse.Service) {
	m.sse = s
	if m.inAppService != nil {
		m.inAppService.SetSSE(s)
	}
}

// InAppService exposes the in-app notification service for integration points.
func (m *Module) InAppService() *inapp.Service { return m.inAppService }

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

// SetOrganizationMemberReader injects a reader for org members.
func (m *Module) SetOrganizationMemberReader(reader OrganizationMemberReader) {
	m.orgMemberReader = reader
}

// SetLeadTimelineWriter injects the lead timeline writer.
func (m *Module) SetLeadTimelineWriter(writer LeadTimelineWriter) { m.leadTimeline = writer }

// SetQuotePDFGenerator injects the PDF generator for pre-generating unsigned PDFs on quote send.
func (m *Module) SetQuotePDFGenerator(gen QuotePDFGenerator) { m.quotePDFGen = gen }

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
	if m.settingsReader == nil {
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
		if len(m.smtpEncryptionKey) == 0 {
			return nil, fmt.Errorf("smtp password is configured but SMTP_ENCRYPTION_KEY is not set")
		}
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
	bus.Subscribe(events.LeadAssigned{}.EventName(), m)
	bus.Subscribe(events.LeadDataChanged{}.EventName(), m)
	bus.Subscribe(events.PipelineStageChanged{}.EventName(), m)
	bus.Subscribe(events.ManualInterventionRequired{}.EventName(), m)

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
	case events.LeadAssigned:
		return m.handleLeadAssigned(ctx, e)
	case events.LeadDataChanged:
		return m.handleLeadDataChanged(ctx, e)
	case events.PipelineStageChanged:
		return m.handlePipelineStageChanged(ctx, e)
	case events.ManualInterventionRequired:
		return m.handleManualInterventionRequired(ctx, e)
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

func renderTemplateText(tpl string, data map[string]any) (string, error) {
	normalizedTpl := tpl
	if !strings.Contains(tpl, "{{.") {
		normalizedTpl = normalizeFrontendTemplateSyntax(tpl, data)
	}
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

func normalizeFrontendTemplateSyntax(tpl string, data map[string]any) string {
	return frontendPlaceholderPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		submatches := frontendPlaceholderPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		path := strings.TrimSpace(submatches[1])
		canonical := canonicalizeTemplatePath(path, data)
		return "{{." + canonical + "}}"
	})
}

func canonicalizeTemplatePath(path string, data map[string]any) string {
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return path
	}

	resolved := make([]string, 0, len(segments))
	var current any = data

	for _, segment := range segments {
		resolvedSegment := segment
		next, ok := findCaseInsensitiveMapValue(current, segment)
		if ok {
			resolvedSegment = next.key
			current = next.value
		} else {
			current = nil
		}
		resolved = append(resolved, resolvedSegment)
	}

	return strings.Join(resolved, ".")
}

type mapValueMatch struct {
	key   string
	value any
}

func findCaseInsensitiveMapValue(current any, key string) (mapValueMatch, bool) {
	currentMap, ok := current.(map[string]any)
	if !ok {
		return mapValueMatch{}, false
	}

	if value, ok := currentMap[key]; ok {
		return mapValueMatch{key: key, value: value}, true
	}

	for candidateKey, candidateValue := range currentMap {
		if strings.EqualFold(candidateKey, key) {
			return mapValueMatch{key: candidateKey, value: candidateValue}, true
		}
	}

	return mapValueMatch{}, false
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
			TenantID: dispatchCtx.Exec.OrgID,
			Kind:     "whatsapp",
			Template: "whatsapp_send",
			Payload:  payload,
			RunAt:    dispatchCtx.RunAt,
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
	text := normalizeEscapedLineBreaks(strings.TrimSpace(*raw))
	if text == "" {
		return "", nil
	}
	rendered, err := renderTemplateText(text, vars)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered), nil
}

func normalizeEscapedLineBreaks(value string) string {
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\r`, "\n",
	)
	return replacer.Replace(value)
}

func renderWorkflowTemplateText(rule *workflowRule, vars map[string]any) string {
	if rule == nil || rule.TemplateText == nil {
		return ""
	}

	rendered, err := renderStepTemplate(rule.TemplateText, vars)
	if err != nil {
		return ""
	}

	return rendered
}

func renderWorkflowTemplateTextWithError(rule *workflowRule, vars map[string]any) (string, error) {
	if rule == nil || rule.TemplateText == nil {
		return "", nil
	}
	return renderStepTemplate(rule.TemplateText, vars)
}

func renderWorkflowTemplateSubject(rule *workflowRule, vars map[string]any) string {
	if rule == nil || rule.TemplateSubject == nil {
		return ""
	}

	rendered, err := renderStepTemplate(rule.TemplateSubject, vars)
	if err != nil {
		return ""
	}

	return rendered
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
	acceptURL := m.buildPublicURL("/partner-offer", e.PublicToken)

	// Build WhatsApp draft URL
	priceFormatted := formatCurrencyEURCents(e.VakmanPriceCents)
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
		if err := m.offerTimeline.WriteOfferEvent(ctx, PartnerOfferTimelineEventParams{
			LeadID:    e.LeadID,
			ServiceID: &serviceID,
			OrgID:     e.OrganizationID,
			ActorType: "System",
			ActorName: "Offer Dispatch",
			EventType: "partner_offer_created",
			Title:     "Werkaanbod verstuurd naar vakman",
			Summary:   &summary,
			Metadata: map[string]any{
				"offerId":          e.OfferID.String(),
				"partnerId":        e.PartnerID.String(),
				"partnerName":      e.PartnerName,
				"vakmanPriceCents": e.VakmanPriceCents,
				"publicToken":      e.PublicToken,
				"acceptanceUrl":    acceptURL,
				"whatsappUrl":      whatsappURL,
				"drafts":           drafts,
			},
		}); err != nil {
			m.log.Error("failed to write partner offer timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	templateVars := map[string]any{
		"partner": map[string]any{
			"name":  e.PartnerName,
			"phone": e.PartnerPhone,
			"email": e.PartnerEmail,
		},
		"offer": map[string]any{
			"id":             e.OfferID.String(),
			"price":          priceFormatted,
			"priceFormatted": priceFormatted,
			"priceCents":     e.VakmanPriceCents,
		},
		"links": map[string]any{
			"accept": acceptURL,
		},
	}

	emailRule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "partner_offer_created", "email", "partner", nil)
	_ = m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         emailRule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    &e.LeadServiceID,
		PartnerEmail: e.PartnerEmail,
		Trigger:      "partner_offer_created",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email werkaanbod verstuurd naar %s", e.PartnerName),
		FallbackNote: "failed to enqueue partner_offer_created partner email workflow",
	})

	whatsAppRule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "partner_offer_created", "whatsapp", "partner", nil)
	if whatsAppRule != nil && whatsAppRule.Enabled && strings.TrimSpace(e.PartnerPhone) != "" {
		messageText, err := renderWorkflowTemplateTextWithError(whatsAppRule, templateVars)
		if err != nil {
			m.log.Warn(msgWorkflowWhatsAppTemplateRenderFailed, "orgId", e.OrganizationID, "trigger", "partner_offer_created", "audience", "partner", "error", err)
			return nil
		}
		if strings.TrimSpace(messageText) == "" {
			return nil
		}
		steps := []repository.WorkflowStep{{
			Enabled:      true,
			Channel:      "whatsapp",
			Audience:     "partner",
			DelayMinutes: whatsAppRule.DelayMinutes,
			TemplateBody: &messageText,
			RecipientConfig: map[string]any{
				"includePartner": true,
			},
		}}
		_ = m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
			OrgID:          e.OrganizationID,
			LeadID:         &e.LeadID,
			ServiceID:      &e.LeadServiceID,
			PartnerPhone:   e.PartnerPhone,
			PartnerEmail:   e.PartnerEmail,
			Trigger:        "partner_offer_created",
			DefaultSummary: fmt.Sprintf("WhatsApp werkaanbod verstuurd naar %s", e.PartnerName),
			DefaultActor:   "System",
			DefaultOrigin:  workflowEngineActorName,
		})
	}

	return nil
}

func (m *Module) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetAppBaseURL(), "/")
	return base + path + "?token=" + tokenValue
}

func (m *Module) buildPublicURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	return base + path + "/" + tokenValue
}

// ── Partner offer event handlers ────────────────────────────────────────

const (
	partnerOfferNotificationEmail = "info@salestainable.nl"
	partnerOfferCreatedTemplate   = "Hallo %s,\n\nEr is een nieuw werkaanbod voor u beschikbaar ter waarde van %s.\n\nBekijk het aanbod en geef uw beschikbaarheid door via onderstaande link:\n%s\n\nMet vriendelijke groet"
	leadWelcomeSummaryFmt         = "WhatsApp welkomstbericht verstuurd naar %s"
	defaultOrgNameFallback        = "ons team"

	msgWorkflowWhatsAppTemplateRenderFailed = "workflow whatsapp template render failed"
	msgWorkflowEmailDispatchSkipped         = "workflow email dispatch skipped"
	msgWorkflowWhatsAppDispatchSkipped      = "workflow whatsapp dispatch skipped"

	invalidOutboxPayloadPrefix = "invalid payload: "
	maxOutboxRetryAttempts     = 5
	workflowEngineActorName    = "Workflow Engine"
	quotePublicPathPrefix      = "/quote/"
	quotePDFPathFmt            = "%s/api/v1/public/quotes/%s/pdf"
	outboxRetryBaseDelay       = time.Minute
	outboxRetryMaxDelay        = 60 * time.Minute
)

var operationsNotificationRoles = map[string]struct{}{
	"admin": {},
	"agent": {},
	"scout": {},
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func normalizeWhatsAppMessage(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}

	// Common rich-text editor HTML (Quill) -> line breaks.
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n",
		"<p>", "",
		"</div>", "\n",
		"<div>", "",
	)
	text = replacer.Replace(text)

	// Remove any remaining tags.
	text = htmlTagPattern.ReplaceAllString(text, "")

	// Decode entities (&nbsp; etc.).
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")

	// Normalize whitespace while preserving newlines.
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		trimmed := strings.Join(strings.Fields(line), " ")
		if trimmed == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
			cleaned = append(cleaned, "")
			continue
		}
		blankCount = 0
		cleaned = append(cleaned, trimmed)
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

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
		if err := m.offerTimeline.WriteOfferEvent(ctx, PartnerOfferTimelineEventParams{
			LeadID:    e.LeadID,
			ServiceID: &serviceID,
			OrgID:     e.OrganizationID,
			ActorType: "Partner",
			ActorName: e.PartnerName,
			EventType: "partner_offer_accepted",
			Title:     "Werkaanbod geaccepteerd",
			Summary:   &summary,
			Metadata: map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
			},
		}); err != nil {
			m.log.Error("failed to write partner offer accepted timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	// 2. Send notification email to configured address (fallback to default)
	notificationEmail := m.resolvePartnerOfferNotificationEmail(ctx, e.OrganizationID)
	if err := m.sender.SendPartnerOfferAcceptedEmail(ctx, notificationEmail, e.PartnerName, e.OfferID.String()); err != nil {
		m.log.Error("failed to send partner offer accepted email",
			"offerId", e.OfferID,
			"toEmail", notificationEmail,
			"error", err,
		)
		// Non-fatal: continue to send confirmation to vakman
	}
	m.log.Info("partner offer accepted email sent",
		"offerId", e.OfferID,
		"toEmail", notificationEmail,
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
		if err := m.offerTimeline.WriteOfferEvent(ctx, PartnerOfferTimelineEventParams{
			LeadID:    e.LeadID,
			ServiceID: &serviceID,
			OrgID:     e.OrganizationID,
			ActorType: "Partner",
			ActorName: e.PartnerName,
			EventType: "partner_offer_rejected",
			Title:     "Werkaanbod afgewezen",
			Summary:   &summary,
			Metadata: map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
				"reason":      e.Reason,
				"drafts":      drafts,
			},
		}); err != nil {
			m.log.Error("failed to write partner offer rejected timeline event",
				"offerId", e.OfferID,
				"error", err,
			)
		}
	}

	// 2. Send notification email to configured address (fallback to default)
	notificationEmail := m.resolvePartnerOfferNotificationEmail(ctx, e.OrganizationID)
	if err := m.sender.SendPartnerOfferRejectedEmail(ctx, notificationEmail, e.PartnerName, e.OfferID.String(), e.Reason); err != nil {
		m.log.Error("failed to send partner offer rejected email",
			"offerId", e.OfferID,
			"toEmail", notificationEmail,
			"error", err,
		)
		return err
	}
	m.log.Info("partner offer rejected email sent",
		"offerId", e.OfferID,
		"toEmail", notificationEmail,
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
		if err := m.offerTimeline.WriteOfferEvent(ctx, PartnerOfferTimelineEventParams{
			LeadID:    e.LeadID,
			ServiceID: &serviceID,
			OrgID:     e.OrganizationID,
			ActorType: "System",
			ActorName: "Offer Expiry",
			EventType: "partner_offer_expired",
			Title:     "Werkaanbod verlopen",
			Summary:   &summary,
			Metadata: map[string]any{
				"offerId":     e.OfferID.String(),
				"partnerId":   e.PartnerID.String(),
				"partnerName": e.PartnerName,
				"drafts":      drafts,
			},
		}); err != nil {
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
	m.log.Info("outbox record processed successfully", "outboxId", rec.ID.String(), "kind", rec.Kind, "template", rec.Template)

	return nil
}

func (m *Module) handleOutboxDeliveryError(ctx context.Context, rec notificationoutbox.Record, deliveryErr error) {
	attempt := rec.Attempts + 1
	if attempt >= maxOutboxRetryAttempts {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, deliveryErr.Error())
		m.log.Warn("notification outbox exhausted retries",
			"outboxId", rec.ID.String(),
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
			"outboxId", rec.ID.String(),
			"attempt", attempt,
			"error", err,
		)
		return
	}

	m.log.Warn("notification outbox scheduled retry",
		"outboxId", rec.ID.String(),
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
		m.log.Debug("outbox record already succeeded; skipping", "outboxId", rec.ID.String())
		return rec, false, nil
	}
	if err := m.notificationOutbox.MarkProcessing(ctx, rec.ID); err != nil {
		return notificationoutbox.Record{}, false, err
	}
	m.log.Debug("outbox record marked processing", "outboxId", rec.ID.String(), "kind", rec.Kind, "template", rec.Template)
	return rec, true, nil
}

func (m *Module) processGenericWhatsAppOutbox(ctx context.Context, e events.NotificationOutboxDue, rec notificationoutbox.Record) error {
	var payload whatsAppSendOutboxPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, invalidOutboxPayloadPrefix+err.Error())
		return nil
	}
	if strings.TrimSpace(payload.PhoneNumber) == "" {
		m.log.Debug("outbox whatsapp payload has no phone number; marking succeeded", "outboxId", rec.ID.String())
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
	if leadID != nil {
		if m.leadWhatsAppReader == nil {
			// If we cannot verify opt-in, treat as a transient failure so the outbox can retry
			// rather than silently dropping the notification.
			return fmt.Errorf("leadWhatsAppReader not configured")
		}
		optedIn, err := m.leadWhatsAppReader.IsWhatsAppOptedIn(ctx, *leadID, orgID)
		if err != nil {
			m.log.Warn(
				"failed to resolve lead whatsapp opt-in for outbox; will retry",
				"outboxId", rec.ID.String(),
				"leadId", *leadID,
				"orgId", orgID,
				"error", err,
			)
			return err
		}
		if !optedIn {
			m.log.Info("lead opted out; skipping whatsapp outbox send", "outboxId", rec.ID.String(), "leadId", *leadID, "orgId", orgID)
			_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
			return nil
		}
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
	m.log.Info("whatsapp outbox delivered", "outboxId", rec.ID.String(), "orgId", orgID, "phone", payload.PhoneNumber, "category", payload.Category)
	return nil
}

func (m *Module) processGenericEmailOutbox(ctx context.Context, e events.NotificationOutboxDue, rec notificationoutbox.Record) error {
	var payload emailSendOutboxPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, invalidOutboxPayloadPrefix+err.Error())
		return nil
	}

	if strings.TrimSpace(payload.ToEmail) == "" {
		m.log.Debug("outbox email payload has no recipient; marking succeeded", "outboxId", rec.ID.String())
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
	m.log.Info("email outbox delivered", "outboxId", rec.ID.String(), "orgId", orgID, "toEmail", payload.ToEmail)
	return nil
}

func (m *Module) markOutboxUnsupported(ctx context.Context, rec notificationoutbox.Record) {
	msg := fmt.Sprintf("unsupported outbox kind/template: %s/%s", rec.Kind, rec.Template)
	_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, msg)
	m.log.Warn("unsupported outbox record", "outboxId", rec.ID.String(), "kind", rec.Kind, "template", rec.Template)
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
	base := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/track/%s", base, publicToken)
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

func (m *Module) handleManualInterventionRequired(ctx context.Context, e events.ManualInterventionRequired) error {
	m.notifyOrgMembersInAppByRoles(ctx, e.TenantID, operationsNotificationRoles, inapp.SendParams{
		Title:        "Handmatige interventie vereist",
		Content:      "Geautomatiseerde verwerking vereist menselijke beoordeling.",
		ResourceID:   &e.LeadID,
		ResourceType: "lead",
		Category:     "warning",
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

	// Pre-generate the unsigned PDF so download links in workflows resolve instantly.
	if m.quotePDFGen != nil {
		if _, _, err := m.quotePDFGen.RegeneratePDF(ctx, e.QuoteID, e.OrganizationID); err != nil {
			m.log.Warn("failed to pre-generate quote PDF on send", "quoteId", e.QuoteID, "error", err)
		}
	}

	_ = m.dispatchQuoteSentLeadEmailWorkflow(ctx, e)
	_ = m.dispatchQuoteSentLeadWhatsAppWorkflow(ctx, e)

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
	TemplateVars map[string]any
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteEmailWorkflow(ctx context.Context, p dispatchQuoteEmailWorkflowParams) bool {
	if p.Rule == nil {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_not_found")
		return false
	}
	if !p.Rule.Enabled {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_disabled")
		return true
	}
	if strings.TrimSpace(p.LeadEmail) == "" && strings.TrimSpace(p.PartnerEmail) == "" {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "no_recipients")
		return true
	}
	if m.notificationOutbox == nil {
		m.log.Warn("workflow email dispatch blocked", "orgId", p.OrgID, "trigger", p.Trigger, "reason", "outbox_repo_missing")
		return false
	}

	bodyText, bodyErr := renderWorkflowTemplateTextWithError(p.Rule, p.TemplateVars)
	subjectText, subjectErr := renderWorkflowTemplateSubjectWithError(p.Rule, p.TemplateVars)
	if bodyErr != nil || subjectErr != nil {
		m.log.Warn("workflow email template render failed",
			"orgId", p.OrgID,
			"trigger", p.Trigger,
			"bodyError", bodyErr,
			"subjectError", subjectErr,
		)
		return true
	}
	subject := strings.TrimSpace(subjectText)
	if subject == "" || strings.TrimSpace(bodyText) == "" {
		m.log.Info(msgWorkflowEmailDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "empty_subject_or_body", "hasSubject", subject != "", "hasBody", strings.TrimSpace(bodyText) != "")
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
	TemplateVars map[string]any
	Summary      string
	FallbackNote string
}

func (m *Module) dispatchQuoteWhatsAppWorkflow(ctx context.Context, p dispatchQuoteWhatsAppWorkflowParams) bool {
	if p.Rule == nil {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_not_found")
		return false
	}
	if !p.Rule.Enabled {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "rule_disabled")
		return true
	}
	if strings.TrimSpace(p.LeadPhone) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "missing_phone")
		return true
	}
	if p.LeadID != nil && !m.isLeadWhatsAppOptedIn(ctx, *p.LeadID, p.OrgID) {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "leadId", *p.LeadID, "reason", "lead_opted_out_in_db")
		return true
	}
	if m.notificationOutbox == nil {
		m.log.Warn("workflow whatsapp dispatch blocked", "orgId", p.OrgID, "trigger", p.Trigger, "reason", "outbox_repo_missing")
		return false
	}

	messageText, err := renderWorkflowTemplateTextWithError(p.Rule, p.TemplateVars)
	if err != nil {
		m.log.Warn(msgWorkflowWhatsAppTemplateRenderFailed, "orgId", p.OrgID, "trigger", p.Trigger, "error", err)
		return true
	}
	messageText = normalizeWhatsAppMessage(messageText)
	if strings.TrimSpace(messageText) == "" {
		m.log.Info(msgWorkflowWhatsAppDispatchSkipped, "orgId", p.OrgID, "trigger", p.Trigger, "reason", "empty_message_body")
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
	enqueueErr := m.enqueueWorkflowSteps(ctx, steps, workflowStepExecutionContext{
		OrgID:          p.OrgID,
		LeadID:         p.LeadID,
		ServiceID:      p.ServiceID,
		LeadPhone:      p.LeadPhone,
		Trigger:        p.Trigger,
		DefaultSummary: p.Summary,
		DefaultActor:   "System",
		DefaultOrigin:  workflowEngineActorName,
	})
	if enqueueErr != nil {
		m.log.Warn(p.FallbackNote, "error", enqueueErr, "orgId", p.OrgID)
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

func (m *Module) dispatchQuoteSentLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteSent) bool {
	if strings.TrimSpace(e.ConsumerPhone) == "" {
		return true
	}
	if !m.isLeadWhatsAppOptedIn(ctx, e.LeadID, e.OrganizationID) {
		return true
	}

	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_sent", "whatsapp", "lead", nil)
	proposalURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/") + quotePublicPathPrefix + e.PublicToken
	downloadURL := fmt.Sprintf(quotePDFPathFmt, strings.TrimRight(m.cfg.GetPublicBaseURL(), "/"), e.PublicToken)
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "previewUrl": proposalURL, "downloadUrl": downloadURL},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)

	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_sent",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("WhatsApp offerte verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_sent lead whatsapp workflow",
	})
}

func (m *Module) dispatchQuoteSentLeadEmailWorkflow(ctx context.Context, e events.QuoteSent) bool {
	if strings.TrimSpace(e.ConsumerEmail) == "" {
		return true
	}

	proposalURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/") + quotePublicPathPrefix + e.PublicToken
	downloadURL := fmt.Sprintf(quotePDFPathFmt, strings.TrimRight(m.cfg.GetPublicBaseURL(), "/"), e.PublicToken)
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "previewUrl": proposalURL, "downloadUrl": downloadURL},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_sent", "email", "lead", nil)

	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_sent",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerte verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_sent lead email workflow",
	})
}

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
	nlLoc, _ := time.LoadLocation("Europe/Amsterdam")
	localStart := p.StartTime.In(nlLoc)
	dateStr := localStart.Format("02-01-2006")
	timeStr := localStart.Format("15:04")
	details := m.resolveLeadDetails(ctx, *p.LeadID, p.OrgID)
	templateVars := map[string]any{
		"lead":        map[string]any{"name": name, "phone": p.ConsumerPhone, "email": p.ConsumerEmail},
		"appointment": map[string]any{"date": dateStr, "time": timeStr, "location": strings.TrimSpace(p.Location)},
	}
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
	nlLoc, _ := time.LoadLocation("Europe/Amsterdam")
	localStart := p.StartTime.In(nlLoc)
	dateStr := localStart.Format("02-01-2006")
	timeStr := localStart.Format("15:04")
	details := m.resolveLeadDetails(ctx, *p.LeadID, p.OrgID)
	templateVars := map[string]any{
		"lead":        map[string]any{"name": name, "phone": p.ConsumerPhone, "email": p.ConsumerEmail},
		"appointment": map[string]any{"date": dateStr, "time": timeStr, "location": strings.TrimSpace(p.Location)},
	}
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

func formatCurrencyEURCents(cents int64) string {
	sign := ""
	abs := cents
	if cents < 0 {
		sign = "-"
		abs = -cents
	}
	return fmt.Sprintf("%s€%d,%02d", sign, abs/100, abs%100)
}

func (m *Module) resolvePartnerOfferNotificationEmail(ctx context.Context, orgID uuid.UUID) string {
	if m.settingsReader == nil {
		return partnerOfferNotificationEmail
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to resolve partner offer notification email", "orgId", orgID, "error", err)
		return partnerOfferNotificationEmail
	}

	if settings.NotificationEmail != nil {
		email := strings.TrimSpace(*settings.NotificationEmail)
		if email != "" {
			return email
		}
	}

	return partnerOfferNotificationEmail
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
	m.notifyOrgMembersInAppByRoles(ctx, e.OrganizationID, operationsNotificationRoles, inapp.SendParams{
		Title:        "Offerte geaccepteerd",
		Content:      fmt.Sprintf("%s heeft offerte %s geaccepteerd.", defaultName(strings.TrimSpace(e.ConsumerName), "Klant"), e.QuoteNumber),
		ResourceID:   &e.QuoteID,
		ResourceType: "quote",
		Category:     "success",
	})
	m.publishQuoteAcceptedSSE(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_accepted",
		"Offerte geaccepteerd door "+e.SignatureName,
		map[string]interface{}{"signatureName": e.SignatureName, "totalCents": e.TotalCents, "consumerName": e.ConsumerName})

	m.log.Info("quote accepted event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) dispatchQuoteAcceptedLeadEmailWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	baseURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	downloadURL := fmt.Sprintf(quotePDFPathFmt, baseURL, e.PublicToken)
	viewURL := baseURL + quotePublicPathPrefix + e.PublicToken
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice, "downloadUrl": downloadURL},
		"links": map[string]any{"view": viewURL, "download": downloadURL},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email bevestiging offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted lead email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedAgentEmailWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.AgentName), "adviseur")
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"partner": map[string]any{"name": name, "email": e.AgentEmail},
		"lead":    map[string]any{"name": defaultName(strings.TrimSpace(e.ConsumerName), "de klant")},
		"quote":   map[string]any{"number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice},
		"org":     map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "email", "partner", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		PartnerEmail: e.AgentEmail,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerteacceptatie verstuurd naar %s", name),
		FallbackNote: "failed to enqueue quote_accepted partner email workflow",
	})
}

func (m *Module) dispatchQuoteAcceptedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteAccepted) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	baseURL := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	downloadURL := fmt.Sprintf(quotePDFPathFmt, baseURL, e.PublicToken)
	formattedPrice := formatCurrencyEURCents(e.TotalCents)
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead":  map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{"number": e.QuoteNumber, "totalCents": e.TotalCents, "total": formattedPrice, "totalFormatted": formattedPrice, "downloadUrl": downloadURL},
		"links": map[string]any{"download": downloadURL},
		"org":   map[string]any{"name": e.OrganizationName},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_accepted", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_accepted",
		TemplateVars: templateVars,
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
	m.notifyOrgMembersInAppByRoles(ctx, e.OrganizationID, operationsNotificationRoles, inapp.SendParams{
		Title:        "Offerte afgewezen",
		Content:      fmt.Sprintf("%s heeft offerte afgewezen.", defaultName(strings.TrimSpace(e.ConsumerName), "Klant")),
		ResourceID:   &e.QuoteID,
		ResourceType: "quote",
		Category:     "warning",
	})

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
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead": map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{
			"reason": e.Reason,
		},
		"org": map[string]any{"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback)},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "email", "lead", nil)
	return m.dispatchQuoteEmailWorkflow(ctx, dispatchQuoteEmailWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadEmail:    e.ConsumerEmail,
		Trigger:      "quote_rejected",
		TemplateVars: templateVars,
		Summary:      fmt.Sprintf("Email offerteafwijzing bevestigd naar %s", name),
		FallbackNote: "failed to enqueue quote_rejected lead email workflow",
	})
}

func (m *Module) dispatchQuoteRejectedLeadWhatsAppWorkflow(ctx context.Context, e events.QuoteRejected) bool {
	name := defaultName(strings.TrimSpace(e.ConsumerName), "klant")
	details := m.resolveLeadDetails(ctx, e.LeadID, e.OrganizationID)
	templateVars := map[string]any{
		"lead": map[string]any{"name": name, "phone": e.ConsumerPhone, "email": e.ConsumerEmail},
		"quote": map[string]any{
			"reason": e.Reason,
		},
		"org": map[string]any{"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback)},
	}
	enrichLeadVars(templateVars, details)
	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, e.LeadID, "quote_rejected", "whatsapp", "lead", nil)
	return m.dispatchQuoteWhatsAppWorkflow(ctx, dispatchQuoteWhatsAppWorkflowParams{
		Rule:         rule,
		OrgID:        e.OrganizationID,
		LeadID:       &e.LeadID,
		ServiceID:    e.LeadServiceID,
		LeadPhone:    e.ConsumerPhone,
		Trigger:      "quote_rejected",
		TemplateVars: templateVars,
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

func (m *Module) notifyOrgMembersInAppByRoles(ctx context.Context, orgID uuid.UUID, allowedRoles map[string]struct{}, p inapp.SendParams) {
	if m.inAppService == nil || m.orgMemberReader == nil {
		return
	}

	members, err := m.orgMemberReader.ListOrgMembers(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to list org members for in-app notification", "error", err, "orgId", orgID)
		return
	}

	for _, member := range members {
		if !memberMatchesRoles(member, allowedRoles) {
			continue
		}
		params := p
		params.OrgID = orgID
		params.UserID = member.ID
		_ = m.inAppService.Send(ctx, params)
	}
}

func memberMatchesRoles(member leadrepo.OrgMember, allowedRoles map[string]struct{}) bool {
	if len(allowedRoles) == 0 {
		return true
	}
	if len(member.Roles) == 0 {
		return false
	}
	for _, role := range member.Roles {
		normalized := strings.ToLower(strings.TrimSpace(role))
		if _, ok := allowedRoles[normalized]; ok {
			return true
		}
	}
	return false
}

// Compile-time check that Module implements http.Module.
var _ apphttp.Module = (*Module)(nil)
