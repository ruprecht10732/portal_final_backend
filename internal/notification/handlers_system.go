package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/notification/inapp"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (m *Module) handleNewEmailReceived(ctx context.Context, e events.NewEmailReceived) error {
	if m.inAppService == nil || m.tenancyReader == nil {
		return nil
	}

	orgID, err := m.tenancyReader.GetUserOrganizationID(ctx, e.UserID)
	if err != nil {
		return err
	}

	from := strings.TrimSpace(e.FromAddress)
	if from == "" {
		from = "Onbekende afzender"
	}

	return m.inAppService.Send(ctx, inapp.SendParams{
		OrgID:        orgID,
		UserID:       e.UserID,
		Title:        "Nieuwe e-mail ontvangen",
		Content:      fmt.Sprintf("Van: %s\nOnderwerp: %s", from, e.Subject),
		ResourceID:   &e.AccountID,
		ResourceType: "imap_account",
		Category:     "info",
	})
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

			return fmt.Errorf("leadWhatsAppReader not configured")
		}
		optedIn, err := m.leadWhatsAppReader.IsWhatsAppOptedIn(ctx, *leadID, orgID)
		if err != nil {
			return m.handleWhatsAppOutboxLeadLookupError(ctx, rec, *leadID, orgID, err)
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

	attachments, err := m.resolveEmailOutboxAttachments(ctx, orgID, payload)
	if err != nil {
		if errors.Is(err, errInvalidOutboxPayload) {
			_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, invalidOutboxPayloadPrefix+err.Error())
			return nil
		}
		return err
	}

	sender := m.resolveSender(ctx, orgID)
	if err := sender.SendCustomEmail(ctx, payload.ToEmail, payload.Subject, payload.BodyHTML, attachments...); err != nil {
		return err
	}

	_ = m.notificationOutbox.MarkSucceeded(ctx, rec.ID)
	m.log.Info("email outbox delivered", "outboxId", rec.ID.String(), "orgId", orgID, "toEmail", payload.ToEmail)
	return nil
}

func (m *Module) resolveEmailOutboxAttachments(ctx context.Context, orgID uuid.UUID, payload emailSendOutboxPayload) ([]email.Attachment, error) {
	if len(payload.Attachments) == 0 {
		return nil, nil
	}

	attachments := make([]email.Attachment, 0, len(payload.Attachments))
	for _, spec := range payload.Attachments {
		attachment, err := m.resolveEmailAttachment(ctx, orgID, spec)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (m *Module) markOutboxUnsupported(ctx context.Context, rec notificationoutbox.Record) {
	msg := fmt.Sprintf("unsupported outbox kind/template: %s/%s", rec.Kind, rec.Template)
	_ = m.notificationOutbox.MarkFailed(ctx, rec.ID, msg)
	m.log.Warn("unsupported outbox record", "outboxId", rec.ID.String(), "kind", rec.Kind, "template", rec.Template)
}
