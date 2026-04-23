package notification

import (
	"context"
	"fmt"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/smtpcrypto"
	"strings"
	"time"

	"github.com/google/uuid"
)

// cachedSender holds a resolved email.Sender with a TTL for cache expiry.
type cachedSender struct {
	sender    email.Sender
	expiresAt time.Time
}

// resolveSender returns a tenant-specific SMTPSender if the organization has SMTP
// configured, falling back to the default (Brevo) sender. Results are cached with a
// 5-minute TTL to avoid repeated DB lookups and decryption.
func (m *Module) resolveSender(ctx context.Context, orgID uuid.UUID) email.Sender {

	if cached, ok := m.senderCache.Load(orgID); ok {
		entry := cached.(cachedSender)
		if time.Now().Before(entry.expiresAt) {
			return entry.sender
		}
		m.senderCache.Delete(orgID)
	}

	if m.settingsReader == nil {
		return m.sender
	}

	settings, err := m.settingsReader.GetOrganizationSettings(ctx, orgID)
	if err != nil {
		m.log.Warn("failed to fetch org settings for smtp", "error", err, "orgId", orgID)
		return m.sender
	}

	if settings.SMTPHost == nil || *settings.SMTPHost == "" {

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

type emailSendOutboxPayload struct {
	OrgID       string                    `json:"orgId"`
	ToEmail     string                    `json:"toEmail"`
	Subject     string                    `json:"subject"`
	BodyHTML    string                    `json:"bodyHtml"`
	LeadID      *string                   `json:"leadId,omitempty"`
	ServiceID   *string                   `json:"serviceId,omitempty"`
	Attachments []emailSendAttachmentSpec `json:"attachments,omitempty"`
}

type emailSendAttachmentSpec struct {
	Kind        string                           `json:"kind,omitempty"`
	QuoteID     *string                          `json:"quoteId,omitempty"`
	FileKey     string                           `json:"fileKey,omitempty"`
	FileName    string                           `json:"fileName,omitempty"`
	MIMEType    string                           `json:"mimeType,omitempty"`
	ISDESubsidy *isdeSubsidyPDFAttachmentPayload `json:"isdeSubsidy,omitempty"`
}

func (m *Module) buildEmailAttachmentSpecs(dispatchCtx workflowStepDispatchContext) []emailSendAttachmentSpec {
	trigger := strings.ToLower(strings.TrimSpace(dispatchCtx.Exec.Trigger))
	if trigger != "quote_sent" && trigger != "quote_accepted" {
		return nil
	}

	quoteMap, ok := dispatchCtx.Exec.Variables["quote"].(map[string]any)
	if !ok {
		return nil
	}

	quoteIDText, _ := quoteMap["id"].(string)
	quoteIDText = strings.TrimSpace(quoteIDText)
	if quoteIDText == "" {
		return nil
	}

	quoteNumber, _ := quoteMap["number"].(string)
	quoteNumber = strings.TrimSpace(quoteNumber)
	if quoteNumber == "" {
		quoteNumber = "offerte"
	}

	fileKey, _ := quoteMap["pdfFileKey"].(string)
	attachments := []emailSendAttachmentSpec{{
		Kind:     "quote_pdf",
		QuoteID:  &quoteIDText,
		FileKey:  strings.TrimSpace(fileKey),
		FileName: buildQuotePDFAttachmentFileName(quoteNumber),
		MIMEType: pdfMIMEType,
	}}

	if subsidy := buildISDESubsidyAttachmentPayload(dispatchCtx.Exec.Variables, quoteMap); subsidy != nil {
		attachments = append(attachments, emailSendAttachmentSpec{
			Kind:        "isde_subsidy_pdf",
			FileName:    buildISDESubsidyPDFAttachmentFileName(quoteNumber),
			MIMEType:    pdfMIMEType,
			ISDESubsidy: subsidy,
		})
	}

	return attachments
}

func (m *Module) resolveEmailAttachment(ctx context.Context, orgID uuid.UUID, spec emailSendAttachmentSpec) (email.Attachment, error) {
	switch strings.TrimSpace(spec.Kind) {
	case "", "quote_pdf":
		return m.resolveQuotePDFAttachment(ctx, orgID, spec)
	case "isde_subsidy_pdf":
		return m.resolveISDESubsidyPDFAttachment(spec)
	default:
		return email.Attachment{}, fmt.Errorf("%w: unsupported attachment kind %q", errInvalidOutboxPayload, spec.Kind)
	}
}
