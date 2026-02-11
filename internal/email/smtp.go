package email

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	gomail "github.com/wneessen/go-mail"
)

// SMTPSender implements the Sender interface using a direct SMTP connection via go-mail.
// It renders the same HTML templates as BrevoSender but delivers via the tenant's own SMTP server.
type SMTPSender struct {
	host      string
	port      int
	username  string
	password  string
	fromName  string
	fromEmail string
}

// NewSMTPSender creates a new SMTPSender with the given SMTP credentials.
func NewSMTPSender(host string, port int, username, password, fromEmail, fromName string) *SMTPSender {
	return &SMTPSender{
		host:      host,
		port:      port,
		username:  username,
		password:  password,
		fromName:  fromName,
		fromEmail: fromEmail,
	}
}

func (s *SMTPSender) send(ctx context.Context, toEmail, subject, htmlContent string, attachments ...Attachment) error {
	msg := gomail.NewMsg()
	if err := msg.FromFormat(s.fromName, s.fromEmail); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := msg.To(toEmail); err != nil {
		return fmt.Errorf("smtp to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, htmlContent)

	for _, att := range attachments {
		msg.AttachReader(att.FileName, bytes.NewReader(att.Content))
	}

	client, err := gomail.NewClient(s.host,
		gomail.WithPort(s.port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(s.username),
		gomail.WithPassword(s.password),
		gomail.WithTLSPortPolicy(gomail.TLSOpportunistic),
		gomail.WithTimeout(15*time.Second),
		gomail.WithDialContextFunc(func(dctx context.Context, _ string, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(dctx, "tcp4", addr)
		}),
	)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}

	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}

func (s *SMTPSender) SendVerificationEmail(ctx context.Context, toEmail, verifyURL string) error {
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
	return s.send(ctx, toEmail, subjectVerification, content)
}

func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context, toEmail, resetURL string) error {
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
	return s.send(ctx, toEmail, subjectPasswordReset, content)
}

func (s *SMTPSender) SendVisitInviteEmail(ctx context.Context, toEmail, consumerName, scheduledDate, address string) error {
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
	return s.send(ctx, toEmail, subjectVisitInvite, content)
}

func (s *SMTPSender) SendOrganizationInviteEmail(ctx context.Context, toEmail, organizationName, inviteURL string) error {
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
	return s.send(ctx, toEmail, subject, content)
}

func (s *SMTPSender) SendPartnerInviteEmail(ctx context.Context, toEmail, organizationName, partnerName, inviteURL string) error {
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
	return s.send(ctx, toEmail, subject, content)
}

func (s *SMTPSender) SendQuoteProposalEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber, proposalURL string) error {
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
	return s.send(ctx, toEmail, subject, content)
}

func (s *SMTPSender) SendQuoteAcceptedEmail(ctx context.Context, toEmail, agentName, quoteNumber, consumerName string, totalCents int64) error {
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
	return s.send(ctx, toEmail, subject, content)
}

func (s *SMTPSender) SendQuoteAcceptedThankYouEmail(ctx context.Context, toEmail, consumerName, organizationName, quoteNumber string, attachments ...Attachment) error {
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
	return s.send(ctx, toEmail, subject, content, attachments...)
}

func (s *SMTPSender) SendPartnerOfferAcceptedEmail(ctx context.Context, toEmail, partnerName, offerID string) error {
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
	return s.send(ctx, toEmail, subject, content)
}

func (s *SMTPSender) SendPartnerOfferAcceptedConfirmationEmail(ctx context.Context, toEmail, partnerName string) error {
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
	return s.send(ctx, toEmail, subjectPartnerOfferAcceptedConfirmation, content)
}

func (s *SMTPSender) SendPartnerOfferRejectedEmail(ctx context.Context, toEmail, partnerName, offerID, reason string) error {
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
	return s.send(ctx, toEmail, subject, content)
}
