package notification

import (
	"context"
	"fmt"
	"net/url"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/platform/phone"
	"strings"

	"github.com/google/uuid"
)

// PartnerOfferTimelineEventParams describes the payload for a partner-offer timeline event.
// Kept as a struct to avoid long parameter lists at call sites.
type PartnerOfferTimelineEventParams struct {
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

// PartnerOfferTimelineWriter writes partner-offer events into the leads timeline.
type PartnerOfferTimelineWriter interface {
	WriteOfferEvent(ctx context.Context, params PartnerOfferTimelineEventParams) error
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

	acceptURL := m.buildPublicURL("/partner-offer", e.PublicToken)

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
	whatsappURL := fmt.Sprintf("https://wa.me/%s?text=%s", cleanPhone, url.QueryEscape(whatsappMsg))

	m.log.Info("partner offer created — acceptance URL generated",
		"offerId", e.OfferID,
		"organizationId", e.OrganizationID,
		"partnerId", e.PartnerID,
		"leadServiceId", e.LeadServiceID,
		"vakmanPriceCents", e.VakmanPriceCents,
		"acceptanceUrl", acceptURL,
	)

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
				"phoneNumber":      phone.NormalizeE164(e.PartnerPhone),
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
		"org": map[string]any{
			"name": defaultName(strings.TrimSpace(e.OrganizationName), defaultOrgNameFallback),
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

func (m *Module) handlePartnerOfferAccepted(ctx context.Context, e events.PartnerOfferAccepted) error {
	m.log.Info("partner offer accepted",
		"offerId", e.OfferID,
		"partnerId", e.PartnerID,
		"partnerName", e.PartnerName,
		"leadId", e.LeadID,
	)

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

	notificationEmail := m.resolvePartnerOfferNotificationEmail(ctx, e.OrganizationID)
	if err := m.sender.SendPartnerOfferAcceptedEmail(ctx, notificationEmail, e.PartnerName, e.OfferID.String()); err != nil {
		m.log.Error("failed to send partner offer accepted email",
			"offerId", e.OfferID,
			"toEmail", notificationEmail,
			"error", err,
		)

	}
	m.log.Info("partner offer accepted email sent",
		"offerId", e.OfferID,
		"toEmail", notificationEmail,
	)

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
