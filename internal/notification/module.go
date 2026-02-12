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
	"strings"
	"sync"
	"text/template"
	"time"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/smtpcrypto"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

// QuoteAcceptanceProcessor handles the post-acceptance side effects:
// PDF generation, upload to storage, and persisting the file key.
type QuoteAcceptanceProcessor interface {
	// GenerateAndStorePDF builds the quote PDF, uploads it to storage,
	// and persists the file key. Returns the file key, the raw PDF bytes, or an error.
	GenerateAndStorePDF(ctx context.Context, quoteID, organizationID uuid.UUID, orgName, customerName, signatureName string) (fileKey string, pdfBytes []byte, err error)
}

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

type NotificationWorkflowReader interface {
	ListNotificationWorkflows(ctx context.Context, organizationID uuid.UUID) ([]repository.NotificationWorkflow, error)
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
	pdfProc            QuoteAcceptanceProcessor
	actWriter          QuoteActivityWriter
	offerTimeline      PartnerOfferTimelineWriter
	whatsapp           WhatsAppSender
	leadTimeline       LeadTimelineWriter
	settingsReader     OrganizationSettingsReader
	workflowReader     NotificationWorkflowReader
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

// SetQuoteAcceptanceProcessor injects the processor for PDF generation on quote acceptance.
func (m *Module) SetQuoteAcceptanceProcessor(p QuoteAcceptanceProcessor) { m.pdfProc = p }

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

func (m *Module) SetWorkflowReader(reader NotificationWorkflowReader) {
	m.workflowReader = reader
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

type leadWelcomeOutboxPayload struct {
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	ConsumerName  string `json:"consumerName"`
	ConsumerPhone string `json:"consumerPhone"`
	PublicToken   string `json:"publicToken"`
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

type workflowRule struct {
	Enabled      bool
	DelayMinutes int
	LeadSource   *string
	TemplateText *string
}

func (m *Module) resolveWorkflowRule(ctx context.Context, orgID uuid.UUID, trigger string, channel string, audience string) *workflowRule {
	if m.workflowReader == nil {
		return nil
	}
	workflows, err := m.workflowReader.ListNotificationWorkflows(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to load notification workflows", "error", err, "orgId", orgID)
		return nil
	}
	for _, w := range workflows {
		if w.Trigger == trigger && strings.EqualFold(w.Channel, channel) && strings.EqualFold(w.Audience, audience) {
			return &workflowRule{
				Enabled:      w.Enabled,
				DelayMinutes: w.DelayMinutes,
				LeadSource:   w.LeadSource,
				TemplateText: w.TemplateText,
			}
		}
	}
	return nil
}

func renderTemplateText(tpl string, data map[string]any) (string, error) {
	parsed, err := template.New("msg").Option("missingkey=zero").Parse(tpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := parsed.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
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
	m.log.Info("processing lead created notification", "leadId", e.LeadID)

	if strings.EqualFold(strings.TrimSpace(e.Source), "quote_flow") {
		m.log.Info("lead created from quote flow, skipping welcome message", "leadId", e.LeadID)
		return nil
	}

	rule := m.resolveWorkflowRule(ctx, e.TenantID, "lead_welcome", "whatsapp", "lead")
	if rule != nil {
		if !rule.Enabled {
			m.log.Info("workflow disabled: skipping lead welcome", "leadId", e.LeadID)
			return nil
		}
		if rule.LeadSource != nil && strings.TrimSpace(*rule.LeadSource) != "" {
			if !strings.EqualFold(strings.TrimSpace(*rule.LeadSource), strings.TrimSpace(e.Source)) {
				m.log.Info("workflow lead source mismatch: skipping lead welcome", "leadId", e.LeadID)
				return nil
			}
		}
	}

	if !e.WhatsAppOptedIn {
		m.log.Info("whatsapp disabled for lead, skipping welcome message", "leadId", e.LeadID)
		return nil
	}

	if e.ConsumerPhone == "" {
		return nil
	}

	consumerName := strings.TrimSpace(e.ConsumerName)
	if consumerName == "" {
		consumerName = "daar"
	}

	trackLink := m.buildLeadTrackLink(e.PublicToken)
	message := ""
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
		if err != nil {
			m.log.Warn("failed to render workflow template; using default", "error", err, "trigger", "lead_welcome")
		} else {
			message = rendered
		}
	}
	if strings.TrimSpace(message) == "" {
		message = buildLeadWelcomeMessage(consumerName, trackLink)
	}

	// Preferred: durable scheduling via outbox.
	if m.notificationOutbox != nil {
		delayMinutes := 0
		if rule != nil {
			delayMinutes = rule.DelayMinutes
		} else {
			d := m.resolveLeadWelcomeWhatsAppDelay(ctx, e.TenantID)
			delayMinutes = int(d / time.Minute)
		}
		runAt := time.Now().UTC().Add(time.Duration(delayMinutes) * time.Minute)

		svcID := e.LeadServiceID
		metadata := buildWhatsAppSentMetadata("lead_welcome", "lead", e.ConsumerPhone, message)
		metadata["preferredContactChannel"] = "WhatsApp"
		metadata["suggestedContactMessage"] = message

		payload := whatsAppSendOutboxPayload{
			OrgID:       e.TenantID.String(),
			LeadID:      ptrString(e.LeadID.String()),
			ServiceID:   ptrString(svcID.String()),
			PhoneNumber: e.ConsumerPhone,
			Message:     message,
			Category:    "lead_welcome",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp welkomstbericht verstuurd naar %s", consumerName),
			ActorType:   "System",
			ActorName:   "Portal",
			Metadata:    metadata,
		}

		_, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID: e.TenantID,
			Kind:     "whatsapp",
			Template: "whatsapp_send",
			Payload:  payload,
			RunAt:    runAt,
		})
		if err == nil {
			return nil
		}
		m.log.Warn("failed to enqueue whatsapp lead welcome outbox; falling back to legacy send", "error", err, "leadId", e.LeadID)
	}

	// Fallback: legacy in-process delay.
	go func() {
		bg := context.Background()
		d := m.resolveLeadWelcomeWhatsAppDelay(bg, e.TenantID)
		if rule != nil {
			d = time.Duration(rule.DelayMinutes) * time.Minute
		}
		if d > 0 {
			time.Sleep(d)
		}

		svcID := e.LeadServiceID
		metadata := buildWhatsAppSentMetadata("lead_welcome", "lead", e.ConsumerPhone, message)
		metadata["preferredContactChannel"] = "WhatsApp"
		metadata["suggestedContactChannel"] = "WhatsApp"
		metadata["suggestedContactMessage"] = message
		_ = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
			Ctx:         bg,
			OrgID:       e.TenantID,
			LeadID:      &e.LeadID,
			ServiceID:   &svcID,
			PhoneNumber: e.ConsumerPhone,
			Message:     message,
			Category:    "lead_welcome",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp welkomstbericht verstuurd naar %s", consumerName),
			ActorType:   "System",
			ActorName:   "Portal",
			Metadata:    metadata,
		})
	}()

	return nil
}

func ptrString(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
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
		return nil
	}

	rec, err := m.notificationOutbox.GetByID(ctx, e.OutboxID)
	if err != nil {
		return err
	}
	if rec.Status == notificationoutbox.StatusSucceeded {
		return nil
	}

	if err := m.notificationOutbox.MarkProcessing(ctx, rec.ID); err != nil {
		return err
	}

	if rec.Kind != "whatsapp" {
		msg := fmt.Sprintf("unsupported outbox kind/template: %s/%s", rec.Kind, rec.Template)
		_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, msg)
		return nil
	}

	// New path: generic WhatsApp send.
	if rec.Template == "whatsapp_send" {
		var payload whatsAppSendOutboxPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, "invalid payload: "+err.Error())
			return nil
		}
		if strings.TrimSpace(payload.PhoneNumber) == "" {
			_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
			return nil
		}

		orgID := e.TenantID
		if strings.TrimSpace(payload.OrgID) != "" {
			if parsed, err := uuid.Parse(payload.OrgID); err == nil {
				orgID = parsed
			}
		}

		var leadID *uuid.UUID
		if payload.LeadID != nil {
			if parsed, err := uuid.Parse(*payload.LeadID); err == nil {
				leadID = &parsed
			}
		}
		var svcID *uuid.UUID
		if payload.ServiceID != nil {
			if parsed, err := uuid.Parse(*payload.ServiceID); err == nil {
				svcID = &parsed
			}
		}

		if leadID != nil {
			if !m.isLeadWhatsAppOptedIn(ctx, *leadID, orgID) {
				_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
				return nil
			}
		}

		err = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
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
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, err.Error())
			return err
		}

		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	// Legacy path: lead welcome payload.
	if rec.Template == "lead_welcome" {
		var payload leadWelcomeOutboxPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, "invalid payload: "+err.Error())
			return nil
		}
		if strings.TrimSpace(payload.ConsumerPhone) == "" {
			_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
			return nil
		}

		leadID, err := uuid.Parse(payload.LeadID)
		if err != nil {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, "invalid leadId: "+err.Error())
			return nil
		}
		svcID, err := uuid.Parse(payload.LeadServiceID)
		if err != nil {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, "invalid leadServiceId: "+err.Error())
			return nil
		}

		if !m.isLeadWhatsAppOptedIn(ctx, leadID, e.TenantID) {
			_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
			return nil
		}

		consumerName := strings.TrimSpace(payload.ConsumerName)
		if consumerName == "" {
			consumerName = "daar"
		}

		trackLink := m.buildLeadTrackLink(payload.PublicToken)
		message := buildLeadWelcomeMessage(consumerName, trackLink)

		metadata := buildWhatsAppSentMetadata("lead_welcome", "lead", payload.ConsumerPhone, message)
		metadata["preferredContactChannel"] = "WhatsApp"
		metadata["suggestedContactMessage"] = message

		err = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
			Ctx:         ctx,
			OrgID:       e.TenantID,
			LeadID:      &leadID,
			ServiceID:   &svcID,
			PhoneNumber: payload.ConsumerPhone,
			Message:     message,
			Category:    "lead_welcome",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp welkomstbericht verstuurd naar %s", consumerName),
			ActorType:   "System",
			ActorName:   "Portal",
			Metadata:    metadata,
		})
		if err != nil {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, err.Error())
			return err
		}

		_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
		return nil
	}

	msg := fmt.Sprintf("unsupported outbox kind/template: %s/%s", rec.Kind, rec.Template)
	_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, msg)
	return nil
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

func (m *Module) resolveLeadWelcomeWhatsAppDelay(ctx context.Context, orgID uuid.UUID) time.Duration {
	// Default: 2 minutes.
	d := 2 * time.Minute
	if m.settingsReader == nil {
		return d
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to fetch org settings for whatsapp welcome delay, using default", "error", err, "orgId", orgID)
		return d
	}

	mins := settings.WhatsAppWelcomeDelayMinutes
	if mins < 0 {
		mins = 0
	}
	return time.Duration(mins) * time.Minute
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
	// Send the quote proposal email to the consumer
	if e.ConsumerEmail != "" {
		proposalURL := strings.TrimRight(m.cfg.GetAppBaseURL(), "/") + "/quote/" + e.PublicToken
		sender := m.resolveSender(ctx, e.OrganizationID)
		if err := sender.SendQuoteProposalEmail(ctx, e.ConsumerEmail, e.ConsumerName, e.OrganizationName, e.QuoteNumber, proposalURL); err != nil {
			m.log.Error("failed to send quote proposal email",
				"quoteId", e.QuoteID,
				"email", e.ConsumerEmail,
				"error", err,
			)
			return err
		}
		m.log.Info("quote proposal email sent",
			"quoteId", e.QuoteID,
			"email", e.ConsumerEmail,
			"quoteNumber", e.QuoteNumber,
		)
	} else {
		m.log.Warn("quote sent but no consumer email available, skipping email",
			"quoteId", e.QuoteID,
			"leadId", e.LeadID,
		)
	}

	// Push SSE event so the agent dashboard updates
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteSent, e.QuoteID, map[string]interface{}{
		"quoteNumber": e.QuoteNumber,
		"status":      "Sent",
	})

	if m.sse != nil {
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

	// Persist activity
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_sent",
		"Offerte verstuurd naar "+e.ConsumerName,
		map[string]interface{}{"quoteNumber": e.QuoteNumber, "consumerEmail": e.ConsumerEmail})

	if e.ConsumerPhone != "" {
		if !m.isLeadWhatsAppOptedIn(ctx, e.LeadID, e.OrganizationID) {
			return nil
		}

		rule := m.resolveWorkflowRule(ctx, e.OrganizationID, "quote_sent", "whatsapp", "lead")
		if rule != nil {
			if !rule.Enabled {
				return nil
			}
		}
		proposalURL := strings.TrimRight(m.cfg.GetAppBaseURL(), "/") + "/quote/" + e.PublicToken
		name := strings.TrimSpace(e.ConsumerName)
		if name == "" {
			name = "klant"
		}

		msg := ""
		if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
			rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
				"lead": map[string]any{
					"name":  name,
					"phone": e.ConsumerPhone,
				},
				"quote": map[string]any{
					"number":     e.QuoteNumber,
					"previewUrl": proposalURL,
				},
				"org": map[string]any{
					"name": e.OrganizationName,
				},
			})
			if err == nil {
				msg = rendered
			}
		}
		if strings.TrimSpace(msg) == "" {
			msg = fmt.Sprintf(
				"Hi %s,\n\nUw offerte %s van %s is klaar! 📄\n\nBekijk en accordeer hem direct via deze link:\n%s\n\nMet vriendelijke groet,\n%s",
				name,
				e.QuoteNumber,
				e.OrganizationName,
				proposalURL,
				e.OrganizationName,
			)
		}

		if m.notificationOutbox != nil {
			delayMinutes := 0
			if rule != nil {
				delayMinutes = rule.DelayMinutes
			}
			runAt := time.Now().UTC().Add(time.Duration(delayMinutes) * time.Minute)
			leadID := e.LeadID
			var serviceID *uuid.UUID
			if e.LeadServiceID != nil {
				serviceID = e.LeadServiceID
			}
			payload := whatsAppSendOutboxPayload{
				OrgID:       e.OrganizationID.String(),
				LeadID:      ptrString(leadID.String()),
				ServiceID:   ptrUUIDString(serviceID),
				PhoneNumber: e.ConsumerPhone,
				Message:     msg,
				Category:    "quote_sent",
				Audience:    "lead",
				Summary:     fmt.Sprintf("WhatsApp offerte verstuurd naar %s", name),
				ActorType:   "System",
				ActorName:   "Portal",
			}
			_, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
				TenantID: e.OrganizationID,
				Kind:     "whatsapp",
				Template: "whatsapp_send",
				Payload:  payload,
				RunAt:    runAt,
			})
			if err == nil {
				return nil
			}
		}

		_ = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
			Ctx:         ctx,
			OrgID:       e.OrganizationID,
			LeadID:      &e.LeadID,
			ServiceID:   e.LeadServiceID,
			PhoneNumber: e.ConsumerPhone,
			Message:     msg,
			Category:    "quote_sent",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp offerte verstuurd naar %s", name),
			ActorType:   "System",
			ActorName:   "Portal",
		})
	}

	m.log.Info("quote sent event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) handleAppointmentCreated(ctx context.Context, e events.AppointmentCreated) error {
	if e.Type != "lead_visit" || e.ConsumerPhone == "" || e.LeadID == nil {
		return nil
	}
	if !m.isLeadWhatsAppOptedIn(ctx, *e.LeadID, e.OrganizationID) {
		return nil
	}

	name := strings.TrimSpace(e.ConsumerName)
	if name == "" {
		name = "klant"
	}

	dateStr := e.StartTime.Format("02-01-2006")
	timeStr := e.StartTime.Format("15:04")

	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, "appointment_created", "whatsapp", "lead")
	if rule != nil {
		if !rule.Enabled {
			return nil
		}
	}

	msg := ""
	if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
		rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
			"lead": map[string]any{
				"name":  name,
				"phone": e.ConsumerPhone,
			},
			"appointment": map[string]any{
				"date":     dateStr,
				"time":     timeStr,
				"location": strings.TrimSpace(e.Location),
			},
		})
		if err == nil {
			msg = rendered
		}
	}
	if strings.TrimSpace(msg) == "" {
		msg = fmt.Sprintf(
			"Hi %s,\n\nUw afspraak is bevestigd! ✅\n\nDatum: %s\nTijd: %s\n\nOnze adviseur komt bij u langs voor de opname. Tot dan!",
			name,
			dateStr,
			timeStr,
		)
	}

	if m.notificationOutbox != nil {
		delayMinutes := 0
		if rule != nil {
			delayMinutes = rule.DelayMinutes
		}
		runAt := time.Now().UTC().Add(time.Duration(delayMinutes) * time.Minute)
		payload := whatsAppSendOutboxPayload{
			OrgID:       e.OrganizationID.String(),
			LeadID:      ptrUUIDString(e.LeadID),
			ServiceID:   ptrUUIDString(e.LeadServiceID),
			PhoneNumber: e.ConsumerPhone,
			Message:     msg,
			Category:    "appointment_created",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp afspraakbevestiging verstuurd naar %s", name),
			ActorType:   "System",
			ActorName:   "Portal",
		}
		_, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID: e.OrganizationID,
			Kind:     "whatsapp",
			Template: "whatsapp_send",
			Payload:  payload,
			RunAt:    runAt,
		})
		if err == nil {
			return nil
		}
	}

	_ = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
		Ctx:         ctx,
		OrgID:       e.OrganizationID,
		LeadID:      e.LeadID,
		ServiceID:   e.LeadServiceID,
		PhoneNumber: e.ConsumerPhone,
		Message:     msg,
		Category:    "appointment_created",
		Audience:    "lead",
		Summary:     fmt.Sprintf("WhatsApp afspraakbevestiging verstuurd naar %s", name),
		ActorType:   "System",
		ActorName:   "Portal",
	})
	return nil
}

func (m *Module) handleAppointmentReminderDue(ctx context.Context, e events.AppointmentReminderDue) error {
	if e.Type != "lead_visit" || e.ConsumerPhone == "" || e.LeadID == nil {
		return nil
	}
	if !m.isLeadWhatsAppOptedIn(ctx, *e.LeadID, e.OrganizationID) {
		return nil
	}

	name := strings.TrimSpace(e.ConsumerName)
	if name == "" {
		name = "klant"
	}

	dateStr := e.StartTime.Format("02-01-2006")
	timeStr := e.StartTime.Format("15:04")

	rule := m.resolveWorkflowRule(ctx, e.OrganizationID, "appointment_reminder", "whatsapp", "lead")
	if rule != nil {
		if !rule.Enabled {
			return nil
		}
	}

	msg := ""
	if rule != nil && rule.TemplateText != nil && strings.TrimSpace(*rule.TemplateText) != "" {
		rendered, err := renderTemplateText(*rule.TemplateText, map[string]any{
			"lead": map[string]any{
				"name":  name,
				"phone": e.ConsumerPhone,
			},
			"appointment": map[string]any{
				"date":     dateStr,
				"time":     timeStr,
				"location": strings.TrimSpace(e.Location),
			},
		})
		if err == nil {
			msg = rendered
		}
	}
	if strings.TrimSpace(msg) == "" {
		msg = fmt.Sprintf(
			"Herinnering, %s! ⏰\n\nMorgen staat uw afspraak gepland.\n\nDatum: %s\nTijd: %s\n\nTot morgen!",
			name,
			dateStr,
			timeStr,
		)
	}

	if m.notificationOutbox != nil {
		delayMinutes := 0
		if rule != nil {
			delayMinutes = rule.DelayMinutes
		}
		runAt := time.Now().UTC().Add(time.Duration(delayMinutes) * time.Minute)
		payload := whatsAppSendOutboxPayload{
			OrgID:       e.OrganizationID.String(),
			LeadID:      ptrUUIDString(e.LeadID),
			ServiceID:   ptrUUIDString(e.LeadServiceID),
			PhoneNumber: e.ConsumerPhone,
			Message:     msg,
			Category:    "appointment_reminder",
			Audience:    "lead",
			Summary:     fmt.Sprintf("WhatsApp afspraakherinnering verstuurd naar %s", name),
			ActorType:   "System",
			ActorName:   "Portal",
		}
		_, err := m.notificationOutbox.Insert(ctx, notificationoutbox.InsertParams{
			TenantID: e.OrganizationID,
			Kind:     "whatsapp",
			Template: "whatsapp_send",
			Payload:  payload,
			RunAt:    runAt,
		})
		if err == nil {
			return nil
		}
	}

	_ = m.sendWhatsAppBestEffort(whatsAppBestEffortParams{
		Ctx:         ctx,
		OrgID:       e.OrganizationID,
		LeadID:      e.LeadID,
		ServiceID:   e.LeadServiceID,
		PhoneNumber: e.ConsumerPhone,
		Message:     msg,
		Category:    "appointment_reminder",
		Audience:    "lead",
		Summary:     fmt.Sprintf("WhatsApp afspraakherinnering verstuurd naar %s", name),
		ActorType:   "System",
		ActorName:   "Portal",
	})
	return nil
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
	pdfBytes := m.generateAcceptancePDF(ctx, e)
	m.sendAcceptanceThankYouEmail(ctx, e, pdfBytes)
	m.sendAcceptanceAgentEmail(ctx, e)
	m.publishQuoteAcceptedSSE(e)
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_accepted",
		"Offerte geaccepteerd door "+e.SignatureName,
		map[string]interface{}{"signatureName": e.SignatureName, "totalCents": e.TotalCents, "consumerName": e.ConsumerName})

	m.log.Info("quote accepted event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) generateAcceptancePDF(ctx context.Context, e events.QuoteAccepted) []byte {
	if m.pdfProc == nil {
		return nil
	}

	fileKey, generatedPDF, err := m.pdfProc.GenerateAndStorePDF(ctx, e.QuoteID, e.OrganizationID, e.OrganizationName, e.ConsumerName, e.SignatureName)
	if err != nil {
		m.log.Error("failed to generate/store acceptance PDF",
			"quoteId", e.QuoteID,
			"error", err,
		)
		return nil
	}

	m.log.Info("acceptance PDF generated and stored",
		"quoteId", e.QuoteID,
		"fileKey", fileKey,
	)

	return generatedPDF
}

func (m *Module) sendAcceptanceThankYouEmail(ctx context.Context, e events.QuoteAccepted, pdfBytes []byte) {
	if e.ConsumerEmail == "" {
		return
	}

	var attachments []email.Attachment
	if len(pdfBytes) > 0 {
		attachments = append(attachments, email.Attachment{
			Content:  pdfBytes,
			FileName: "offerte-" + e.QuoteNumber + ".pdf",
			MIMEType: "application/pdf",
		})
	}

	sender := m.resolveSender(ctx, e.OrganizationID)
	if err := sender.SendQuoteAcceptedThankYouEmail(ctx, e.ConsumerEmail, e.ConsumerName, e.OrganizationName, e.QuoteNumber, attachments...); err != nil {
		m.log.Error("failed to send acceptance thank-you email to customer",
			"quoteId", e.QuoteID,
			"email", e.ConsumerEmail,
			"error", err,
		)
		return
	}

	m.log.Info("acceptance thank-you email sent to customer",
		"quoteId", e.QuoteID,
		"email", e.ConsumerEmail,
	)
}

func (m *Module) sendAcceptanceAgentEmail(ctx context.Context, e events.QuoteAccepted) {
	if e.AgentEmail == "" {
		return
	}

	sender := m.resolveSender(ctx, e.OrganizationID)
	if err := sender.SendQuoteAcceptedEmail(ctx, e.AgentEmail, e.AgentName, e.QuoteNumber, e.ConsumerName, e.TotalCents); err != nil {
		m.log.Error("failed to send acceptance notification email to agent",
			"quoteId", e.QuoteID,
			"email", e.AgentEmail,
			"error", err,
		)
		return
	}

	m.log.Info("acceptance notification email sent to agent",
		"quoteId", e.QuoteID,
		"email", e.AgentEmail,
	)
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
