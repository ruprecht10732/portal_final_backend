// Package notification provides event handlers for sending notifications
// (emails, SMS, push, etc.) in response to domain events.
// This module subscribes to events and inverts the dependency: domain modules
// no longer need to know about email providers or templates.
package notification

import (
	"context"
	"fmt"
	"strings"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"

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
	SendMessage(ctx context.Context, phoneNumber string, message string) error
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

// Module handles all notification-related event subscriptions.
type Module struct {
	sender        email.Sender
	cfg           config.NotificationConfig
	log           *logger.Logger
	sse           *sse.Service
	pdfProc       QuoteAcceptanceProcessor
	actWriter     QuoteActivityWriter
	offerTimeline PartnerOfferTimelineWriter
	whatsapp      WhatsAppSender
	leadTimeline  LeadTimelineWriter
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

// SetLeadTimelineWriter injects the lead timeline writer.
func (m *Module) SetLeadTimelineWriter(writer LeadTimelineWriter) { m.leadTimeline = writer }

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

	// Quote domain events
	bus.Subscribe(events.QuoteSent{}.EventName(), m)
	bus.Subscribe(events.QuoteViewed{}.EventName(), m)
	bus.Subscribe(events.QuoteUpdatedByCustomer{}.EventName(), m)
	bus.Subscribe(events.QuoteAnnotated{}.EventName(), m)
	bus.Subscribe(events.QuoteAccepted{}.EventName(), m)
	bus.Subscribe(events.QuoteRejected{}.EventName(), m)

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
	default:
		m.log.Warn("unhandled event type", "event", event.EventName())
		return nil
	}
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
	if err := m.sender.SendPartnerInviteEmail(ctx, e.Email, e.OrganizationName, e.PartnerName, inviteURL); err != nil {
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
		if err := m.sender.SendPartnerOfferAcceptedConfirmationEmail(ctx, e.PartnerEmail, e.PartnerName); err != nil {
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
	_ = ctx
	m.log.Info("processing lead created notification", "leadId", e.LeadID)

	if m.whatsapp == nil || e.ConsumerPhone == "" {
		return nil
	}

	consumerName := strings.TrimSpace(e.ConsumerName)
	if consumerName == "" {
		consumerName = "daar"
	}

	message := fmt.Sprintf(
		"Beste %s,\n\n"+
			"Bedankt voor je aanvraag! 👍\n\n"+
			"We hebben alles ontvangen en gaan het nu rustig doornemen. "+
			"Vandaag nemen we contact met je op om het verder te bespreken.",
		consumerName,
	)

	go func() {
		bg := context.Background()
		if err := m.whatsapp.SendMessage(bg, e.ConsumerPhone, message); err != nil {
			m.log.Error("failed to send welcome whatsapp", "error", err, "leadId", e.LeadID)
			return
		}

		if m.leadTimeline == nil {
			return
		}

		summary := fmt.Sprintf("WhatsApp welkomstbericht verstuurd naar %s", consumerName)
		if err := m.leadTimeline.CreateTimelineEvent(bg, LeadTimelineEventParams{
			LeadID:    e.LeadID,
			ServiceID: nil,
			OrgID:     e.TenantID,
			ActorType: "System",
			ActorName: "Portal",
			EventType: "whatsapp_sent",
			Title:     "WhatsApp verstuurd",
			Summary:   &summary,
			Metadata: map[string]any{
				"preferredContactChannel": "WhatsApp",
				"suggestedContactMessage": message,
				"messageLanguage":         "nl",
				"messageAudience":         "lead",
				"messageCategory":         "lead_welcome",
			},
		}); err != nil {
			m.log.Error("failed to write whatsapp timeline event", "error", err, "leadId", e.LeadID)
		}
	}()

	return nil
}

// ── Quote event handlers ────────────────────────────────────────────────

func (m *Module) handleQuoteSent(ctx context.Context, e events.QuoteSent) error {
	// Send the quote proposal email to the consumer
	if e.ConsumerEmail != "" {
		proposalURL := strings.TrimRight(m.cfg.GetAppBaseURL(), "/") + "/quote/" + e.PublicToken
		if err := m.sender.SendQuoteProposalEmail(ctx, e.ConsumerEmail, e.ConsumerName, e.OrganizationName, e.QuoteNumber, proposalURL); err != nil {
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

	// Persist activity
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_sent",
		"Offerte verstuurd naar "+e.ConsumerName,
		map[string]interface{}{"quoteNumber": e.QuoteNumber, "consumerEmail": e.ConsumerEmail})

	m.log.Info("quote sent event processed", "quoteId", e.QuoteID)
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
	// 1. Generate PDF, upload to MinIO, and persist file key
	var pdfBytes []byte
	if m.pdfProc != nil {
		fileKey, generatedPDF, err := m.pdfProc.GenerateAndStorePDF(ctx, e.QuoteID, e.OrganizationID, e.OrganizationName, e.ConsumerName, e.SignatureName)
		if err != nil {
			m.log.Error("failed to generate/store acceptance PDF",
				"quoteId", e.QuoteID,
				"error", err,
			)
			// Non-fatal: continue with emails and SSE even if PDF fails
		} else {
			pdfBytes = generatedPDF
			m.log.Info("acceptance PDF generated and stored",
				"quoteId", e.QuoteID,
				"fileKey", fileKey,
			)
		}
	}

	// 2. Send "thank you" email to customer — with PDF attached if available
	if e.ConsumerEmail != "" {
		var attachments []email.Attachment
		if len(pdfBytes) > 0 {
			attachments = append(attachments, email.Attachment{
				Content:  pdfBytes,
				FileName: "offerte-" + e.QuoteNumber + ".pdf",
				MIMEType: "application/pdf",
			})
		}
		if err := m.sender.SendQuoteAcceptedThankYouEmail(ctx, e.ConsumerEmail, e.ConsumerName, e.OrganizationName, e.QuoteNumber, attachments...); err != nil {
			m.log.Error("failed to send acceptance thank-you email to customer",
				"quoteId", e.QuoteID,
				"email", e.ConsumerEmail,
				"error", err,
			)
		} else {
			m.log.Info("acceptance thank-you email sent to customer",
				"quoteId", e.QuoteID,
				"email", e.ConsumerEmail,
			)
		}
	}

	// 3. Send acceptance notification email to the agent
	if e.AgentEmail != "" {
		if err := m.sender.SendQuoteAcceptedEmail(ctx, e.AgentEmail, e.AgentName, e.QuoteNumber, e.ConsumerName, e.TotalCents); err != nil {
			m.log.Error("failed to send acceptance notification email to agent",
				"quoteId", e.QuoteID,
				"email", e.AgentEmail,
				"error", err,
			)
		} else {
			m.log.Info("acceptance notification email sent to agent",
				"quoteId", e.QuoteID,
				"email", e.AgentEmail,
			)
		}
	}

	// 4. Push SSE event so the agent dashboard updates in real time
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteAccepted, e.QuoteID, map[string]interface{}{
		"signatureName": e.SignatureName,
		"totalCents":    e.TotalCents,
	})

	// 5. Persist activity
	m.logQuoteActivity(ctx, e.QuoteID, e.OrganizationID, "quote_accepted",
		"Offerte geaccepteerd door "+e.SignatureName,
		map[string]interface{}{"signatureName": e.SignatureName, "totalCents": e.TotalCents, "consumerName": e.ConsumerName})

	m.log.Info("quote accepted event processed", "quoteId", e.QuoteID)
	return nil
}

func (m *Module) handleQuoteRejected(ctx context.Context, e events.QuoteRejected) error {
	m.pushQuoteSSE(e.OrganizationID, sse.EventQuoteRejected, e.QuoteID, map[string]interface{}{
		"reason": e.Reason,
	})
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
	}
}
