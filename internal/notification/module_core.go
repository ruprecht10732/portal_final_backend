package notification

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"html/template"
	"time"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/identity/repository"
	leadrepo "portal_final_backend/internal/leads/repository"
	notificationdb "portal_final_backend/internal/notification/db"
	notifhandler "portal_final_backend/internal/notification/handler"
	"portal_final_backend/internal/notification/inapp"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrganizationSettingsReader provides org-level settings for notifications.
type OrganizationSettingsReader interface {
	GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (repository.OrganizationSettings, error)
}

// UserTenancyReader resolves organization membership for users.
type UserTenancyReader interface {
	GetUserOrganizationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// OrganizationMemberReader lists organization users for fan-out notifications.
type OrganizationMemberReader interface {
	ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]leadrepo.OrgMember, error)
}

type cachedOrgName struct {
	name      string
	expiresAt time.Time
}

// Module handles all notification-related event subscriptions.
type Module struct {
	pool                *pgxpool.Pool
	sender              email.Sender
	cfg                 config.NotificationConfig
	log                 *logger.Logger
	sse                 *sse.Service
	actWriter           QuoteActivityWriter
	offerTimeline       PartnerOfferTimelineWriter
	quotePDFGen         QuotePDFGenerator
	quotePDFStorage     QuotePDFFileStorage
	quotePDFBucket      string
	quotePDFScheduler   QuoteAcceptedPDFScheduler
	subsidyPDFGen       SubsidyPDFGenerator
	whatsapp            WhatsAppSender
	whatsAppInboxWriter WhatsAppInboxWriter
	leadTimeline        LeadTimelineWriter
	settingsReader      OrganizationSettingsReader
	tenancyReader       UserTenancyReader
	workflowResolver    WorkflowResolver
	leadWhatsAppReader  LeadWhatsAppReader
	orgMemberReader     OrganizationMemberReader
	leadAssigneeReader  LeadAssigneeReader
	notificationOutbox  *notificationoutbox.Repository
	inAppService        *inapp.Service
	inAppHandler        *notifhandler.HTTPHandler
	smtpEncryptionKey   []byte
	senderCache         sync.Map // map[uuid.UUID]cachedSender
	orgNameCache        sync.Map // map[uuid.UUID]cachedOrgName
	quoteViewedDebounce sync.Map // map[uuid.UUID]time.Time
	queries             *notificationdb.Queries
}

// New creates a new notification module.
func New(pool *pgxpool.Pool, sender email.Sender, cfg config.NotificationConfig, log *logger.Logger) *Module {
	inAppRepo := inapp.NewRepository(pool)
	inAppSvc := inapp.NewService(inAppRepo, log)
	var queries *notificationdb.Queries
	if pool != nil {
		queries = notificationdb.New(pool)
	}

	return &Module{
		pool:          pool,
		sender:        sender,
		cfg:           cfg,
		log:           log,
		queries:       queries,
		subsidyPDFGen: subsidyPDFGeneratorFunc(generateISDESubsidyPDF),
		inAppService:  inAppSvc,
		inAppHandler:  notifhandler.NewHTTPHandler(inAppSvc),
	}
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

// SetWhatsAppInboxWriter injects a persistence hook for sent WhatsApp messages.
func (m *Module) SetWhatsAppInboxWriter(writer WhatsAppInboxWriter) { m.whatsAppInboxWriter = writer }

// SetOrganizationSettingsReader injects org settings reader for WhatsApp device resolution.
func (m *Module) SetOrganizationSettingsReader(reader OrganizationSettingsReader) {
	m.settingsReader = reader
}

// SetUserTenancyReader injects user-to-organization lookup for in-app fanout.
func (m *Module) SetUserTenancyReader(reader UserTenancyReader) {
	m.tenancyReader = reader
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

// SetLeadAssigneeReader injects a reader for lead assignee lookup.
func (m *Module) SetLeadAssigneeReader(reader LeadAssigneeReader) {
	m.leadAssigneeReader = reader
}

// SetLeadTimelineWriter injects the lead timeline writer.
func (m *Module) SetLeadTimelineWriter(writer LeadTimelineWriter) { m.leadTimeline = writer }

// SetQuotePDFGenerator injects the PDF generator for pre-generating unsigned PDFs on quote send.
func (m *Module) SetQuotePDFGenerator(gen QuotePDFGenerator) { m.quotePDFGen = gen }

// SetQuotePDFStorage injects storage access for already-generated quote PDFs.
func (m *Module) SetQuotePDFStorage(storage QuotePDFFileStorage, bucket string) {
	m.quotePDFStorage = storage
	m.quotePDFBucket = strings.TrimSpace(bucket)
}

// SetQuoteAcceptedPDFScheduler injects async PDF task enqueueing for accepted quotes.
func (m *Module) SetQuoteAcceptedPDFScheduler(scheduler QuoteAcceptedPDFScheduler) {
	m.quotePDFScheduler = scheduler
}

// SetSubsidyPDFGenerator injects the generator for ISDE subsidy summary attachments.
func (m *Module) SetSubsidyPDFGenerator(gen SubsidyPDFGenerator) { m.subsidyPDFGen = gen }

// SetSMTPEncryptionKey sets the AES key used to decrypt SMTP passwords from org settings.
func (m *Module) SetSMTPEncryptionKey(key []byte) { m.smtpEncryptionKey = key }

// SetNotificationOutbox injects the notification outbox repository.
func (m *Module) SetNotificationOutbox(repo *notificationoutbox.Repository) {
	m.notificationOutbox = repo
}

// RegisterHandlers subscribes to all relevant domain events on the event bus.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {

	bus.Subscribe(events.UserSignedUp{}.EventName(), m)
	bus.Subscribe(events.EmailVerificationRequested{}.EventName(), m)
	bus.Subscribe(events.PasswordResetRequested{}.EventName(), m)

	bus.Subscribe(events.OrganizationInviteCreated{}.EventName(), m)

	bus.Subscribe(events.PartnerInviteCreated{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferCreated{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferAccepted{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferRejected{}.EventName(), m)
	bus.Subscribe(events.PartnerOfferExpired{}.EventName(), m)

	bus.Subscribe(events.LeadCreated{}.EventName(), m)
	bus.Subscribe(events.LeadAssigned{}.EventName(), m)
	bus.Subscribe(events.LeadDataChanged{}.EventName(), m)
	bus.Subscribe(events.PipelineStageChanged{}.EventName(), m)
	bus.Subscribe(events.ManualInterventionRequired{}.EventName(), m)

	bus.Subscribe(events.QuoteSent{}.EventName(), m)
	bus.Subscribe(events.QuoteViewed{}.EventName(), m)
	bus.Subscribe(events.QuoteUpdatedByCustomer{}.EventName(), m)
	bus.Subscribe(events.QuoteAnnotated{}.EventName(), m)
	bus.Subscribe(events.QuoteAccepted{}.EventName(), m)
	bus.Subscribe(events.QuoteRejected{}.EventName(), m)

	bus.Subscribe(events.AppointmentCreated{}.EventName(), m)
	bus.Subscribe(events.AppointmentReminderDue{}.EventName(), m)
	bus.Subscribe(events.NotificationOutboxDue{}.EventName(), m)

	bus.Subscribe(events.NewEmailReceived{}.EventName(), m)

	m.log.Info("notification module registered event handlers")
}

func (m *Module) purgeCaches() {
	now := time.Now()
	
	m.senderCache.Range(func(key, value any) bool {
		if entry, ok := value.(cachedSender); ok && now.After(entry.expiresAt) {
			m.senderCache.Delete(key)
		}
		return true
	})

	m.orgNameCache.Range(func(key, value any) bool {
		if entry, ok := value.(cachedOrgName); ok && now.After(entry.expiresAt) {
			m.orgNameCache.Delete(key)
		}
		return true
	})

	m.quoteViewedDebounce.Range(func(key, value any) bool {
		if lastSentAt, ok := value.(time.Time); ok && now.Sub(lastSentAt) > 60*time.Minute {
			m.quoteViewedDebounce.Delete(key)
		}
		return true
	})
}

// Handle routes events to the appropriate handler method.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	if time.Now().UnixNano()%100 == 0 {
		m.purgeCaches()
	}

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
	case events.NewEmailReceived:
		return m.handleNewEmailReceived(ctx, e)
	default:
		m.log.Warn("unhandled event type", "event", event.EventName())
		return nil
	}
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

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	text, _ := values[key].(string)
	return text
}

func stringFromNestedMap(values map[string]any, key string, nestedKey string) string {
	nested, ok := values[key].(map[string]any)
	if !ok {
		return ""
	}
	return stringFromMap(nested, nestedKey)
}

func renderStepTemplate(raw *string, vars map[string]any) (string, error) {
	if raw == nil {
		return "", nil
	}
	text := normalizeEscapedLineBreaks(strings.TrimSpace(*raw))
	if text == "" {
		return "", nil
	}
	rendered, err := renderTemplateText(text, mergeWorkflowTemplateVars(buildWorkflowStepVariables(workflowStepExecutionContext{}), vars))
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

var adminOnlyRoles = map[string]struct{}{
	"admin": {},
}

func ptrUUIDString(v *uuid.UUID) *string {
	if v == nil {
		return nil
	}
	s := v.String()
	return &s
}

var errInvalidOutboxPayload = errors.New("invalid outbox payload")

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

// buildSchedulingLink builds a /track/:token link from leadDetails.
func (m *Module) buildSchedulingLink(d *leadDetails) string {
	if d == nil {
		return ""
	}
	return m.buildLeadTrackLink(d.PublicToken)
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

func (m *Module) handlePipelineStageChanged(ctx context.Context, e events.PipelineStageChanged) error {
	if m.sse != nil {
		m.sse.PublishToLead(e.LeadID, sse.Event{
			Type:      sse.EventLeadStatusChanged,
			LeadID:    e.LeadID,
			ServiceID: e.LeadServiceID,
			Data: map[string]interface{}{
				"oldStage": e.OldStage,
				"newStage": e.NewStage,
			},
		})
	}

	if strings.EqualFold(e.NewStage, "Completed") {
		m.dispatchJobCompletedWorkflows(ctx, e)
	}

	return nil
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

// truncate shortens a string to max characters, appending "…" when truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
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

func (m *Module) sendToAgentOrAdmins(ctx context.Context, orgID uuid.UUID, leadID uuid.UUID, p inapp.SendParams) {
	if m.inAppService == nil {
		return
	}

	if m.leadAssigneeReader != nil {
		agentID, err := m.leadAssigneeReader.GetAssignedAgentID(ctx, leadID, orgID)
		if err != nil {
			m.log.Warn("failed to resolve lead assignee for in-app notification", "error", err, "leadId", leadID, "orgId", orgID)
		} else if agentID != nil {
			params := p
			params.OrgID = orgID
			params.UserID = *agentID
			_ = m.inAppService.Send(ctx, params)
			return
		}
	}

	m.notifyOrgMembersInAppByRoles(ctx, orgID, adminOnlyRoles, p)
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
