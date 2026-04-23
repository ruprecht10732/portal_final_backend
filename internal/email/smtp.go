package email

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	gomail "github.com/wneessen/go-mail"
)

// SMTPSender reordered for memory alignment.
type SMTPSender struct {
	host      string // 16 bytes
	username  string // 16 bytes
	password  string // 16 bytes
	fromName  string // 16 bytes
	fromEmail string // 16 bytes
	port      int    // 8 bytes
}

// NewSMTPSender creates a new SMTPSender with the given SMTP credentials.
func NewSMTPSender(host string, port int, username, password, fromEmail, fromName string) *SMTPSender {
	return &SMTPSender{
		host:      host,
		port:      port,
		username:  username,
		password:  password,
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

// renderAndSend abstracts the template-to-SMTP lifecycle.
func (s *SMTPSender) renderAndSend(ctx context.Context, to, subject, tpl string, data any, atts ...Attachment) error {
	content, err := renderEmailTemplate(tpl, data)
	if err != nil {
		return fmt.Errorf("render %s: %w", tpl, err)
	}
	return s.send(ctx, to, subject, content, atts...)
}

func (s *SMTPSender) SendVerificationEmail(ctx context.Context, to, url string) error {
	return s.renderAndSend(ctx, to, subjectVerification, "verification.html", verificationEmailData{
		baseEmailData: baseEmailData{
			Title: "Bevestig uw e-mailadres", Heading: "Bevestig uw e-mailadres",
			CTALabel: "E-mailadres verifiëren", CTAURL: url,
		},
	})
}

func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context, to, url string) error {
	return s.renderAndSend(ctx, to, subjectPasswordReset, "password_reset.html", passwordResetEmailData{
		baseEmailData: baseEmailData{
			Title: "Wachtwoord opnieuw instellen", Heading: "Wachtwoord opnieuw instellen",
			CTALabel: "Wachtwoord resetten", CTAURL: url,
		},
	})
}

func (s *SMTPSender) SendVisitInviteEmail(ctx context.Context, to, name, date, addr string) error {
	return s.renderAndSend(ctx, to, subjectVisitInvite, "visit_invite.html", visitInviteEmailData{
		baseEmailData: baseEmailData{Title: "Bezoek ingepland", Heading: "Bezoek ingepland"},
		ConsumerName:  name, ScheduledDate: date, Address: addr,
	})
}

func (s *SMTPSender) SendOrganizationInviteEmail(ctx context.Context, to, org, url string) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectOrganizationInviteFmt, org), "organization_invite.html", organizationInviteEmailData{
		baseEmailData:    baseEmailData{Title: inviteTitle, Heading: inviteTitle, CTALabel: "Uitnodiging accepteren", CTAURL: url},
		OrganizationName: org,
	})
}

func (s *SMTPSender) SendPartnerInviteEmail(ctx context.Context, to, org, part, url string) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerInviteFmt, org), "partner_invite.html", partnerInviteEmailData{
		baseEmailData:    baseEmailData{Title: inviteTitle, Heading: inviteTitle, CTALabel: "Uitnodiging bekijken", CTAURL: url},
		OrganizationName: org, PartnerName: part,
	})
}

func (s *SMTPSender) SendQuoteProposalEmail(ctx context.Context, to, cons, org, num, url string) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteProposalFmt, num, org), "quote_proposal.html", quoteProposalEmailData{
		baseEmailData: baseEmailData{Title: "Uw offerte is klaar", Heading: "Uw offerte is klaar", CTALabel: "Bekijk offerte", CTAURL: url},
		ConsumerName:  cons, OrganizationName: org, QuoteNumber: num,
	})
}

func (s *SMTPSender) SendQuoteAcceptedEmail(ctx context.Context, to, agent, num, cons string, total int64) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteAcceptedFmt, num), "quote_accepted.html", quoteAcceptedEmailData{
		baseEmailData: baseEmailData{Title: "Offerte geaccepteerd", Heading: "Offerte geaccepteerd"},
		AgentName:     agent, QuoteNumber: num, ConsumerName: cons, TotalFormatted: formatCurrencyEUR(total),
	})
}

func (s *SMTPSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, to, cons, org, num string, atts ...Attachment) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectQuoteAcceptedThankYouFmt, num), "quote_thank_you.html", quoteAcceptedThankYouEmailData{
		baseEmailData: baseEmailData{Title: "Bedankt voor uw akkoord", Heading: "Bedankt voor uw akkoord"},
		ConsumerName:  cons, OrganizationName: org, QuoteNumber: num, HasAttachments: len(atts) > 0,
	}, atts...)
}

func (s *SMTPSender) SendPartnerOfferAcceptedEmail(ctx context.Context, to, part, id string) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerOfferAcceptedFmt, part), "partner_offer_accepted.html", partnerOfferAcceptedEmailData{
		baseEmailData: baseEmailData{Title: "Werkaanbod geaccepteerd", Heading: "Werkaanbod geaccepteerd"},
		PartnerName:   part, OfferID: id,
	})
}

func (s *SMTPSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, to, part string, atts ...Attachment) error {
	return s.renderAndSend(ctx, to, subjectPartnerOfferAcceptedConfirmation, "partner_offer_confirmation.html", partnerOfferAcceptedConfirmationEmailData{
		baseEmailData: baseEmailData{Title: "Acceptatie bevestigd", Heading: "Acceptatie bevestigd"},
		PartnerName:   part, HasAttachments: len(atts) > 0,
	}, atts...)
}

func (s *SMTPSender) SendPartnerOfferRejectedEmail(ctx context.Context, to, part, id, reason string) error {
	return s.renderAndSend(ctx, to, fmt.Sprintf(subjectPartnerOfferRejectedFmt, part), "partner_offer_rejected.html", partnerOfferRejectedEmailData{
		baseEmailData: baseEmailData{Title: "Werkaanbod afgewezen", Heading: "Werkaanbod afgewezen"},
		PartnerName:   part, OfferID: id, Reason: reason,
	})
}

func (s *SMTPSender) SendCustomEmail(ctx context.Context, to, subject, html string, atts ...Attachment) error {
	return s.send(ctx, to, subject, html, atts...)
}

func (s *SMTPSender) SendDailyDigestEmail(ctx context.Context, to string, data DailyDigestInput) error {
	content, err := renderDailyDigestEmail(data)
	if err != nil {
		return fmt.Errorf("render digest: %w", err)
	}
	return s.send(ctx, to, "Dagelijks overzicht — "+data.OrganizationName, content)
}

// send handles the final SMTP orchestration.
// Complexity: O(A) where A is the number of attachments.
func (s *SMTPSender) send(ctx context.Context, to, subject, html string, atts ...Attachment) error {
	msg := gomail.NewMsg()
	if err := msg.FromFormat(s.fromName, s.fromEmail); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("smtp to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, html)

	for _, att := range atts {
		// AttachReader is O(1) space as it streams the bytes.NewReader into the msg buffer.
		if err := msg.AttachReader(att.FileName, bytes.NewReader(att.Content)); err != nil {
			return fmt.Errorf("smtp attach %s: %w", att.FileName, err)
		}
	}

	client, err := gomail.NewClient(s.host,
		gomail.WithPort(s.port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(s.username),
		gomail.WithPassword(s.password),
		gomail.WithTLSPortPolicy(gomail.TLSOpportunistic),
		gomail.WithTimeout(15*time.Second),
		// Performance: Explicitly use a Dialer with context to prevent I/O blocking.
		gomail.WithDialContextFunc(func(dctx context.Context, _ string, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(dctx, "tcp4", addr)
		}),
	)
	if err != nil {
		return fmt.Errorf("smtp client init: %w", err)
	}

	return client.DialAndSendWithContext(ctx, msg)
}
