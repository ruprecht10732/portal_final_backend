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

type BrevoSender struct {
	apiKey    string
	fromName  string
	fromEmail string
	client    *http.Client
}

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
	subject := "Verifieer uw e-mailadres"
	content := buildEmailTemplate(
		"Bevestig uw e-mailadres",
		"Bedankt voor uw registratie. Verifieer uw e-mailadres om uw account te activeren.",
		"E-mailadres verifiëren",
		verifyURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error {
	subject := "Wachtwoord opnieuw instellen"
	content := buildEmailTemplate(
		"Wachtwoord opnieuw instellen",
		"We hebben een verzoek ontvangen om uw wachtwoord opnieuw in te stellen. Gebruik de link hieronder om een nieuw wachtwoord in te stellen.",
		"Wachtwoord resetten",
		resetURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error {
	subject := "Uw bezoek is ingepland"
	content := buildVisitInviteTemplate(consumerName, scheduledDate, address)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error {
	subject := "U bent uitgenodigd voor " + organizationName
	content := buildEmailTemplate(
		"U bent uitgenodigd",
		"U bent uitgenodigd om lid te worden van "+organizationName+". Klik op de knop hieronder om de uitnodiging te accepteren en uw account aan te maken.",
		"Uitnodiging accepteren",
		inviteURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error {
	subject := "Nieuw werkaanbod van " + organizationName
	content := buildEmailTemplate(
		"U bent uitgenodigd",
		organizationName+" heeft "+partnerName+" uitgenodigd voor een nieuw werkaanbod. Klik op de knop hieronder om de details te bekijken en te reageren.",
		"Uitnodiging bekijken",
		inviteURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteProposalEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber, proposalURL string) error {
	subject := "Offerte " + quoteNumber + " van " + organizationName
	content := buildEmailTemplate(
		"Uw offerte is klaar",
		fmt.Sprintf("Beste %s,<br/><br/>%s heeft offerte %s voor u klaargezet. Bekijk de offerte, selecteer eventuele opties en accepteer of wijs de offerte af.", consumerName, organizationName, quoteNumber),
		"Bekijk offerte",
		proposalURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteAcceptedEmail(ctx context.Context, toEmail, agentName, quoteNumber, consumerName string, totalCents int64) error {
	subject := "Offerte " + quoteNumber + " geaccepteerd"
	content := buildEmailTemplate(
		"Offerte geaccepteerd",
		fmt.Sprintf("Beste %s,<br/><br/>%s heeft offerte %s geaccepteerd (totaal: €%.2f). Bekijk de details in het portaal.", agentName, consumerName, quoteNumber, float64(totalCents)/100),
		"Bekijk details",
		"", // No CTA URL — internal notification
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber string, attachments ...Attachment) error {
	subject := "Bedankt voor uw akkoord — offerte " + quoteNumber

	attachmentNote := ""
	if len(attachments) > 0 {
		attachmentNote = "<br/><br/>In de bijlage vindt u een PDF-kopie van de getekende offerte voor uw administratie."
	}

	content := buildEmailTemplate(
		"Bedankt voor uw akkoord",
		fmt.Sprintf("Beste %s,<br/><br/>Bedankt dat u offerte %s van %s heeft geaccepteerd. Wij nemen zo snel mogelijk contact met u op om de volgende stappen te bespreken.%s<br/><br/>Met vriendelijke groet,<br/>%s", consumerName, quoteNumber, organizationName, attachmentNote, organizationName),
		"",
		"",
	)
	return b.sendWithAttachments(ctx, toEmail, subject, content, attachments...)
}

func (b *BrevoSender) SendPartnerOfferAcceptedEmail(ctx context.Context, toEmail, partnerName, offerID string) error {
	subject := "Werkaanbod geaccepteerd door " + partnerName
	content := buildEmailTemplate(
		"Werkaanbod geaccepteerd",
		fmt.Sprintf("%s heeft het werkaanbod (ID: %s) geaccepteerd en beschikbaarheid doorgegeven.<br/><br/>Bekijk de details en plan de inspectie in via het portaal.", partnerName, offerID),
		"",
		"",
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string) error {
	subject := "Bevestiging: uw acceptatie is ontvangen"
	content := buildEmailTemplate(
		"Acceptatie bevestigd",
		fmt.Sprintf("Beste %s,<br/><br/>Bedankt voor het accepteren van het werkaanbod. Uw beschikbaarheid is ontvangen en wij nemen zo snel mogelijk contact met u op om de inspectie in te plannen.<br/><br/>Met vriendelijke groet", partnerName),
		"",
		"",
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error {
	subject := "Werkaanbod afgewezen door " + partnerName
	reasonText := ""
	if reason != "" {
		reasonText = fmt.Sprintf("<br/><br/>Reden: %s", reason)
	}
	content := buildEmailTemplate(
		"Werkaanbod afgewezen",
		fmt.Sprintf("%s heeft het werkaanbod (ID: %s) afgewezen.%s<br/><br/>U kunt een nieuw aanbod versturen naar een andere vakman via het portaal.", partnerName, offerID, reasonText),
		"",
		"",
	)
	return b.send(ctx, toEmail, subject, content)
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

func buildEmailTemplate(title, message, ctaLabel, ctaURL string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>%s</title>
</head>
<body style="margin:0;padding:0;background:#f4f4f5;font-family:Arial,sans-serif;color:#111827;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:24px 0;">
    <tr>
      <td align="center">
        <table role="presentation" width="520" cellpadding="0" cellspacing="0" style="background:#ffffff;border:1px solid #e4e4e7;padding:24px;">
          <tr>
            <td style="font-size:20px;font-weight:700;text-transform:uppercase;letter-spacing:0.08em;">
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:12px;font-size:14px;line-height:1.5;color:#52525b;">
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:24px;">
              <a href="%s" style="display:inline-block;padding:12px 18px;background:#111827;color:#ffffff;text-decoration:none;text-transform:uppercase;font-size:12px;letter-spacing:0.18em;font-weight:600;">
                %s
              </a>
            </td>
          </tr>
          <tr>
            <td style="padding-top:20px;font-size:12px;color:#a1a1aa;">
							Als de knop niet werkt, kopieer en plak dan deze link in uw browser:<br />
              <a href="%s" style="color:#71717a;">%s</a>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, title, title, message, ctaURL, ctaLabel, ctaURL, ctaURL)
}

func buildVisitInviteTemplate(consumerName, scheduledDate, address string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>Visit Scheduled</title>
</head>
<body style="margin:0;padding:0;background:#f4f4f5;font-family:Arial,sans-serif;color:#111827;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:24px 0;">
    <tr>
      <td align="center">
        <table role="presentation" width="520" cellpadding="0" cellspacing="0" style="background:#ffffff;border:1px solid #e4e4e7;padding:24px;">
          <tr>
            <td style="font-size:20px;font-weight:700;text-transform:uppercase;letter-spacing:0.08em;">
							Bezoek ingepland
            </td>
          </tr>
          <tr>
            <td style="padding-top:12px;font-size:14px;line-height:1.5;color:#52525b;">
							Beste %s,<br /><br />
							Uw bezoek is ingepland op:
            </td>
          </tr>
          <tr>
            <td style="padding-top:16px;font-size:16px;font-weight:600;color:#111827;">
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:8px;font-size:14px;line-height:1.5;color:#52525b;">
							op<br />
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:20px;font-size:12px;color:#a1a1aa;">
							Als u wilt verzetten of vragen heeft, neem dan contact met ons op.
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, consumerName, scheduledDate, address)
}
