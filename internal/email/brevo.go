package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"portal_final_backend/platform/config"
)

type Sender interface {
	SendVerificationEmail(ctx context.Context, toEmail, verifyURL string) error
	SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error
	SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error
	SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error
	SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error
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

type BrevoSender struct {
	apiKey    string
	fromName  string
	fromEmail string
	client    *http.Client
}

type brevoEmailRequest struct {
	Sender struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"sender"`
	To []struct {
		Email string `json:"email"`
	} `json:"to"`
	Subject     string `json:"subject"`
	HTMLContent string `json:"htmlContent"`
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
	subject := "Verify your email"
	content := buildEmailTemplate(
		"Confirm your email",
		"Thanks for signing up. Please verify your email to activate your account.",
		"Verify email",
		verifyURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error {
	subject := "Reset your password"
	content := buildEmailTemplate(
		"Reset your password",
		"We received a request to reset your password. Use the link below to set a new one.",
		"Reset password",
		resetURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error {
	subject := "Your visit has been scheduled"
	content := buildVisitInviteTemplate(consumerName, scheduledDate, address)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error {
	subject := "You're invited to join " + organizationName
	content := buildEmailTemplate(
		"You're invited",
		"You have been invited to join "+organizationName+". Click the button below to accept the invitation and create your account.",
		"Accept invitation",
		inviteURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error {
	subject := "New job invitation from " + organizationName
	content := buildEmailTemplate(
		"You're invited",
		""+organizationName+" invited "+partnerName+" to take on a new job. Click the button below to review the details and respond.",
		"Review invitation",
		inviteURL,
	)
	return b.send(ctx, toEmail, subject, content)
}

func (b *BrevoSender) send(ctx context.Context, toEmail, subject, htmlContent string) error {
	payload := brevoEmailRequest{
		Subject:     subject,
		HTMLContent: htmlContent,
	}
	payload.Sender.Name = b.fromName
	payload.Sender.Email = b.fromEmail
	payload.To = []struct {
		Email string `json:"email"`
	}{{Email: toEmail}}

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
              If the button does not work, copy and paste this link into your browser:<br />
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
              Visit Scheduled
            </td>
          </tr>
          <tr>
            <td style="padding-top:12px;font-size:14px;line-height:1.5;color:#52525b;">
              Dear %s,<br /><br />
              Your visit has been scheduled for:
            </td>
          </tr>
          <tr>
            <td style="padding-top:16px;font-size:16px;font-weight:600;color:#111827;">
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:8px;font-size:14px;line-height:1.5;color:#52525b;">
              at<br />
              %s
            </td>
          </tr>
          <tr>
            <td style="padding-top:20px;font-size:12px;color:#a1a1aa;">
              If you need to reschedule or have any questions, please contact us.
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, consumerName, scheduledDate, address)
}
