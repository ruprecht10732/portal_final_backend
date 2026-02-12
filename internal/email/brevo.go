package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"portal_final_backend/platform/config"
)

// Attachment represents a file attachment for an email.
type Attachment struct {
	Content  []byte // raw file bytes (will be base64-encoded for Brevo)
	FileName string // e.g. "offerte-Q-00042.pdf"
	MIMEType string // e.g. "application/pdf"
}

type Sender interface {
	SendVerificationEmail(ctx context.Context, toEmail, verifyURL string) error
	SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error
	SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error
	SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error
	SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error
	SendQuoteProposalEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber, proposalURL string) error
	SendQuoteAcceptedEmail(ctx context.Context, toEmail, agentName, quoteNumber, consumerName string, totalCents int64) error
	SendQuoteAcceptedThankYouEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber string, attachments ...Attachment) error
	SendPartnerOfferAcceptedEmail(ctx context.Context, toEmail, partnerName, offerID string) error
	SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string) error
	SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error
	SendCustomEmail(ctx context.Context, toEmail, subject, htmlContent string) error
}

type NoopSender struct{}

func (NoopSender) SendVerificationEmail(ctx context.Context, toEmail, verifyURL string) error {
	return nil
}

func (NoopSender) SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error {
	return nil
}

func (NoopSender) SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error {
	return nil
}

func (NoopSender) SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error {
	return nil
}

func (NoopSender) SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error {
	return nil
}

func (NoopSender) SendQuoteProposalEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber, proposalURL string) error {
	return nil
}

func (NoopSender) SendQuoteAcceptedEmail(ctx context.Context, toEmail, agentName, quoteNumber, consumerName string, totalCents int64) error {
	return nil
}

func (NoopSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber string, attachments ...Attachment) error {
	return nil
}

func (NoopSender) SendPartnerOfferAcceptedEmail(ctx context.Context, toEmail, partnerName, offerID string) error {
	return nil
}

func (NoopSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string) error {
	return nil
}

func (NoopSender) SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error {
	return nil
}

func (NoopSender) SendCustomEmail(ctx context.Context, toEmail, subject, htmlContent string) error {
	return nil
}

type BrevoSender struct {
	apiKey    string
	fromName  string
	fromEmail string
	client    *http.Client
}

const inviteTitle = "U bent uitgenodigd"

type brevoAttachment struct {
	Content string `json:"content"` // base64-encoded file content
	Name    string `json:"name"`
}

type brevoEmailRequest struct {
	Sender struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"sender"`
	To []struct {
		Email string `json:"email"`
	} `json:"to"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"htmlContent"`
	Attachment  []brevoAttachment `json:"attachment,omitempty"`
}

func NewSender(cfg config.EmailConfig) (Sender, error) {
	if !cfg.GetEmailEnabled() {
		return NoopSender{}, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return &BrevoSender{
		apiKey:    cfg.GetBrevoAPIKey(),
		fromName:  cfg.GetEmailFromName(),
		fromEmail: cfg.GetEmailFromAddress(),
		client:    client,
	}, nil
}

func (b *BrevoSender) SendVerificationEmail(ctx context.Context, toEmail, verifyURL string) error {
	subject := subjectVerification
	content, err := renderEmailTemplate("verification.html", verificationEmailData{
		baseEmailData: baseEmailData{
			Title:    "Bevestig uw e-mailadres",
			Heading:  "Bevestig uw e-mailadres",
			CTALabel: "E-mailadres verifiëren",
			CTAURL:   verifyURL,
		},
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error {
	subject := subjectPasswordReset
	content, err := renderEmailTemplate("password_reset.html", passwordResetEmailData{
		baseEmailData: baseEmailData{
			Title:    "Wachtwoord opnieuw instellen",
			Heading:  "Wachtwoord opnieuw instellen",
			CTALabel: "Wachtwoord resetten",
			CTAURL:   resetURL,
		},
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error {
	subject := subjectVisitInvite
	content, err := renderEmailTemplate("visit_invite.html", visitInviteEmailData{
		baseEmailData: baseEmailData{
			Title:   "Bezoek ingepland",
			Heading: "Bezoek ingepland",
		},
		ConsumerName:  consumerName,
		ScheduledDate: scheduledDate,
		Address:       address,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error {
	subject := fmt.Sprintf(subjectOrganizationInviteFmt, organizationName)
	content, err := renderEmailTemplate("organization_invite.html", organizationInviteEmailData{
		baseEmailData: baseEmailData{
			Title:    inviteTitle,
			Heading:  inviteTitle,
			CTALabel: "Uitnodiging accepteren",
			CTAURL:   inviteURL,
		},
		OrganizationName: organizationName,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error {
	subject := fmt.Sprintf(subjectPartnerInviteFmt, organizationName)
	content, err := renderEmailTemplate("partner_invite.html", partnerInviteEmailData{
		baseEmailData: baseEmailData{
			Title:    inviteTitle,
			Heading:  inviteTitle,
			CTALabel: "Uitnodiging bekijken",
			CTAURL:   inviteURL,
		},
		OrganizationName: organizationName,
		PartnerName:      partnerName,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteProposalEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber, proposalURL string) error {
	subject := fmt.Sprintf(subjectQuoteProposalFmt, quoteNumber, organizationName)
	content, err := renderEmailTemplate("quote_proposal.html", quoteProposalEmailData{
		baseEmailData: baseEmailData{
			Title:    "Uw offerte is klaar",
			Heading:  "Uw offerte is klaar",
			CTALabel: "Bekijk offerte",
			CTAURL:   proposalURL,
		},
		ConsumerName:     consumerName,
		OrganizationName: organizationName,
		QuoteNumber:      quoteNumber,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteAcceptedEmail(ctx context.Context, toEmail, agentName, quoteNumber, consumerName string, totalCents int64) error {
	subject := fmt.Sprintf(subjectQuoteAcceptedFmt, quoteNumber)
	content, err := renderEmailTemplate("quote_accepted.html", quoteAcceptedEmailData{
		baseEmailData: baseEmailData{
			Title:   "Offerte geaccepteerd",
			Heading: "Offerte geaccepteerd",
		},
		AgentName:      agentName,
		QuoteNumber:    quoteNumber,
		ConsumerName:   consumerName,
		TotalFormatted: formatCurrencyEUR(totalCents),
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber string, attachments ...Attachment) error {
	subject := fmt.Sprintf(subjectQuoteAcceptedThankYouFmt, quoteNumber)
	content, err := renderEmailTemplate("quote_thank_you.html", quoteAcceptedThankYouEmailData{
		baseEmailData: baseEmailData{
			Title:   "Bedankt voor uw akkoord",
			Heading: "Bedankt voor uw akkoord",
		},
		ConsumerName:     consumerName,
		OrganizationName: organizationName,
		QuoteNumber:      quoteNumber,
		HasAttachments:   len(attachments) > 0,
	})
	if err != nil {
		return err
	}
	return b.sendWithAttachments(ctx, toEmail, subject, content, attachments...)
}

func (b *BrevoSender) SendPartnerOfferAcceptedEmail(ctx context.Context, toEmail, partnerName, offerID string) error {
	subject := fmt.Sprintf(subjectPartnerOfferAcceptedFmt, partnerName)
	content, err := renderEmailTemplate("partner_offer_accepted.html", partnerOfferAcceptedEmailData{
		baseEmailData: baseEmailData{
			Title:   "Werkaanbod geaccepteerd",
			Heading: "Werkaanbod geaccepteerd",
		},
		PartnerName: partnerName,
		OfferID:     offerID,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string) error {
	subject := subjectPartnerOfferAcceptedConfirmation
	content, err := renderEmailTemplate("partner_offer_confirmation.html", partnerOfferAcceptedConfirmationEmailData{
		baseEmailData: baseEmailData{
			Title:   "Acceptatie bevestigd",
			Heading: "Acceptatie bevestigd",
		},
		PartnerName: partnerName,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error {
	subject := fmt.Sprintf(subjectPartnerOfferRejectedFmt, partnerName)
	content, err := renderEmailTemplate("partner_offer_rejected.html", partnerOfferRejectedEmailData{
		baseEmailData: baseEmailData{
			Title:   "Werkaanbod afgewezen",
			Heading: "Werkaanbod afgewezen",
		},
		PartnerName: partnerName,
		OfferID:     offerID,
		Reason:      reason,
	})
	if err != nil {
		return err
	}
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendCustomEmail(ctx context.Context, toEmail, subject, htmlContent string) error {
	return b.send(ctx, toEmail, subject, htmlContent)
}

func (b *BrevoSender) send(ctx context.Context, toEmail, subject, htmlContent string) error {
	return b.sendWithAttachments(ctx, toEmail, subject, htmlContent)
}

func (b *BrevoSender) sendWithAttachments(ctx context.Context, toEmail, subject, htmlContent string, attachments ...Attachment) error {
	payload := brevoEmailRequest{
		Subject:     subject,
		HTMLContent: htmlContent,
	}
	payload.Sender.Name = b.fromName
	payload.Sender.Email = b.fromEmail
	payload.To = []struct {
		Email string `json:"email"`
	}{{Email: toEmail}}

	for _, att := range attachments {
		payload.Attachment = append(payload.Attachment, brevoAttachment{
			Content: base64.StdEncoding.EncodeToString(att.Content),
			Name:    att.FileName,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("brevo send failed: status %d: %s", resp.StatusCode, string(data))
	}

	return nil
}
