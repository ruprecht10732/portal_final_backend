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

// Attachment reordered for optimal memory alignment (pointers/slices first).
type Attachment struct {
	Content  []byte // 24 bytes
	FileName string // 16 bytes
	MIMEType string // 16 bytes
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
	SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string, attachments ...Attachment) error
	SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error
	SendCustomEmail(ctx context.Context, toEmail, subject, htmlContent string, attachments ...Attachment) error
	SendDailyDigestEmail(ctx context.Context, toEmail string, data DailyDigestInput) error
}

// DailyDigestInput reordered to minimize struct padding (8-byte fields grouped).
type DailyDigestInput struct {
	StaleLeads                 []DailyDigestStaleLeadInput
	OrganizationName           string
	Date                       string
	DashboardURL               string
	GatekeeperRuns             int
	EstimatorRuns              int
	DispatcherRuns             int
	QuotesGenerated            int
	PhotosAnalyzed             int
	OffersProcessed            int
	PipelineTriage             int
	PipelineNurturing          int
	PipelineEstimation         int
	PipelineProposal           int
	PipelineFulfillment        int
	PipelineManualIntervention int
}

type DailyDigestStaleLeadInput struct {
	ConsumerFirstName string
	ConsumerLastName  string
	ServiceType       string
	PipelineStage     string
	StaleReason       string
}

// ─── Brevo Integration ───────────────────────────────────────────────────────

const (
	brevoAPIURL = "https://api.brevo.com/v3/smtp/email"
	inviteTitle = "U bent uitgenodigd"
)

type brevoSender struct {
	client    *http.Client
	apiKey    string
	fromName  string
	fromEmail string
}

type brevoAttachment struct {
	Content string `json:"content"`
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

	return &brevoSender{
		apiKey:    cfg.GetBrevoAPIKey(),
		fromName:  cfg.GetEmailFromName(),
		fromEmail: cfg.GetEmailFromAddress(),
		client:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// renderAndSend abstracts the repetitive template-to-wire lifecycle.
// This is O(A) where A is the number of attachments.
func (b *brevoSender) renderAndSend(ctx context.Context, to, subject, tpl string, data any, atts ...Attachment) error {
	content, err := renderEmailTemplate(tpl, data)
	if err != nil {
		return fmt.Errorf("render %s: %w", tpl, err)
	}
	return b.send(ctx, to, subject, content, atts...)
}

func (b *brevoSender) SendVerificationEmail(ctx context.Context, to, url string) error {
	return b.renderAndSend(ctx, to, subjectVerification, "verification.html", verificationEmailData{
		baseEmailData: baseEmailData{
			Title: "Bevestig uw e-mailadres", Heading: "Bevestig uw e-mailadres",
			CTALabel: "E-mailadres verifiëren", CTAURL: url,
		},
	})
}

func (b *brevoSender) SendPasswordResetEmail(ctx context.Context, to, url string) error {
	return b.renderAndSend(ctx, to, subjectPasswordReset, "password_reset.html", passwordResetEmailData{
		baseEmailData: baseEmailData{
			Title: "Wachtwoord opnieuw instellen", Heading: "Wachtwoord opnieuw instellen",
			CTALabel: "Wachtwoord resetten", CTAURL: url,
		},
	})
}

func (b *brevoSender) SendVisitInviteEmail(ctx context.Context, to, name, date, addr string) error {
	return b.renderAndSend(ctx, to, subjectVisitInvite, "visit_invite.html", visitInviteEmailData{
		baseEmailData: baseEmailData{Title: "Bezoek ingepland", Heading: "Bezoek ingepland"},
		ConsumerName:  name, ScheduledDate: date, Address: addr,
	})
}

func (b *brevoSender) SendOrganizationInviteEmail(ctx context.Context, to, org, url string) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectOrganizationInviteFmt, org), "organization_invite.html", organizationInviteEmailData{
		baseEmailData:    baseEmailData{Title: inviteTitle, Heading: inviteTitle, CTALabel: "Uitnodiging accepteren", CTAURL: url},
		OrganizationName: org,
	})
}

func (b *brevoSender) SendPartnerInviteEmail(ctx context.Context, to, org, part, url string) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerInviteFmt, org), "partner_invite.html", partnerInviteEmailData{
		baseEmailData:    baseEmailData{Title: inviteTitle, Heading: inviteTitle, CTALabel: "Uitnodiging bekijken", CTAURL: url},
		OrganizationName: org, PartnerName: part,
	})
}

func (b *brevoSender) SendQuoteProposalEmail(ctx context.Context, to, cons, org, num, url string) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteProposalFmt, num, org), "quote_proposal.html", quoteProposalEmailData{
		baseEmailData: baseEmailData{Title: "Uw offerte is klaar", Heading: "Uw offerte is klaar", CTALabel: "Bekijk offerte", CTAURL: url},
		ConsumerName:  cons, OrganizationName: org, QuoteNumber: num,
	})
}

func (b *brevoSender) SendQuoteAcceptedEmail(ctx context.Context, to, agent, num, cons string, total int64) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteAcceptedFmt, num), "quote_accepted.html", quoteAcceptedEmailData{
		baseEmailData: baseEmailData{Title: "Offerte geaccepteerd", Heading: "Offerte geaccepteerd"},
		AgentName:     agent, QuoteNumber: num, ConsumerName: cons, TotalFormatted: formatCurrencyEUR(total),
	})
}

func (b *brevoSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, to, cons, org, num string, atts ...Attachment) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteAcceptedThankYouFmt, num), "quote_thank_you.html", quoteAcceptedThankYouEmailData{
		baseEmailData: baseEmailData{Title: "Bedankt voor uw akkoord", Heading: "Bedankt voor uw akkoord"},
		ConsumerName:  cons, OrganizationName: org, QuoteNumber: num, HasAttachments: len(atts) > 0,
	}, atts...)
}

func (b *brevoSender) SendPartnerOfferAcceptedEmail(ctx context.Context, to, part, id string) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerOfferAcceptedFmt, part), "partner_offer_accepted.html", partnerOfferAcceptedEmailData{
		baseEmailData: baseEmailData{Title: "Werkaanbod geaccepteerd", Heading: "Werkaanbod geaccepteerd"},
		PartnerName:   part, OfferID: id,
	})
}

func (b *brevoSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, to, part string, atts ...Attachment) error {
	return b.renderAndSend(ctx, to, subjectPartnerOfferAcceptedConfirmation, "partner_offer_confirmation.html", partnerOfferAcceptedConfirmationEmailData{
		baseEmailData: baseEmailData{Title: "Acceptatie bevestigd", Heading: "Acceptatie bevestigd"},
		PartnerName:   part, HasAttachments: len(atts) > 0,
	}, atts...)
}

func (b *brevoSender) SendPartnerOfferRejectedEmail(ctx context.Context, to, part, id, reason string) error {
	return b.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerOfferRejectedFmt, part), "partner_offer_rejected.html", partnerOfferRejectedEmailData{
		baseEmailData: baseEmailData{Title: "Werkaanbod afgewezen", Heading: "Werkaanbod afgewezen"},
		PartnerName:   part, OfferID: id, Reason: reason,
	})
}

func (b *brevoSender) SendCustomEmail(ctx context.Context, to, sub, html string, atts ...Attachment) error {
	return b.send(ctx, to, sub, html, atts...)
}

func (b *brevoSender) SendDailyDigestEmail(ctx context.Context, to string, data DailyDigestInput) error {
	content, err := renderDailyDigestEmail(data)
	if err != nil {
		return fmt.Errorf("render digest: %w", err)
	}
	return b.send(ctx, to, "Dagelijks overzicht — "+data.OrganizationName, content)
}

// send handles the final API orchestration.
// O(N) complexity where N is the total size of content + base64(attachments).
func (b *brevoSender) send(ctx context.Context, to, subject, html string, atts ...Attachment) error {
	payload := brevoEmailRequest{
		Subject:     subject,
		HTMLContent: html,
	}
	payload.Sender.Name, payload.Sender.Email = b.fromName, b.fromEmail
	payload.To = []struct {
		Email string `json:"email"`
	}{{Email: to}}

	if len(atts) > 0 {
		payload.Attachment = make([]brevoAttachment, len(atts))
		for i, a := range atts {
			payload.Attachment[i] = brevoAttachment{
				Content: base64.StdEncoding.EncodeToString(a.Content),
				Name:    a.FileName,
			}
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoAPIURL, bytes.NewReader(body))
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
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Security: Use LimitReader to prevent "Body Bomb" memory exhaustion
		// if a third-party API returns a gigabyte of error text.
		lr := io.LimitReader(resp.Body, 1024*1024) // 1MB Limit
		data, _ := io.ReadAll(lr)
		return fmt.Errorf("brevo failure (%d): %s", resp.StatusCode, string(data))
	}

	return nil
}

// ─── Mocks ───────────────────────────────────────────────────────────────────

type NoopSender struct{}

func (NoopSender) SendVerificationEmail(context.Context, string, string) error  { return nil }
func (NoopSender) SendPasswordResetEmail(context.Context, string, string) error { return nil }
func (NoopSender) SendVisitInviteEmail(context.Context, string, string, string, string) error {
	return nil
}
func (NoopSender) SendOrganizationInviteEmail(context.Context, string, string, string) error {
	return nil
}
func (NoopSender) SendPartnerInviteEmail(context.Context, string, string, string, string) error {
	return nil
}
func (NoopSender) SendQuoteProposalEmail(context.Context, string, string, string, string, string) error {
	return nil
}
func (NoopSender) SendQuoteAcceptedEmail(context.Context, string, string, string, string, int64) error {
	return nil
}
func (NoopSender) SendQuoteAcceptedThankYouEmail(context.Context, string, string, string, string, ...Attachment) error {
	return nil
}
func (NoopSender) SendPartnerOfferAcceptedEmail(context.Context, string, string, string) error {
	return nil
}
func (NoopSender) SendPartnerOfferAcceptedConfirmationEmail(context.Context, string, string, ...Attachment) error {
	return nil
}
func (NoopSender) SendPartnerOfferRejectedEmail(context.Context, string, string, string, string) error {
	return nil
}
func (NoopSender) SendCustomEmail(context.Context, string, string, string, ...Attachment) error {
	return nil
}
func (NoopSender) SendDailyDigestEmail(context.Context, string, DailyDigestInput) error { return nil }
