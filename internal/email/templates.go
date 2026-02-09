package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

type baseEmailData struct {
	Title      string
	Heading    string
	Subheading string
	CTALabel   string
	CTAURL     string
}

type verificationEmailData struct {
	baseEmailData
}

type passwordResetEmailData struct {
	baseEmailData
}

type visitInviteEmailData struct {
	baseEmailData
	ConsumerName  string
	ScheduledDate string
	Address       string
}

type organizationInviteEmailData struct {
	baseEmailData
	OrganizationName string
}

type partnerInviteEmailData struct {
	baseEmailData
	OrganizationName string
	PartnerName      string
}

type quoteProposalEmailData struct {
	baseEmailData
	ConsumerName     string
	OrganizationName string
	QuoteNumber      string
}

type quoteAcceptedEmailData struct {
	baseEmailData
	AgentName      string
	QuoteNumber    string
	ConsumerName   string
	TotalFormatted string
}

type quoteAcceptedThankYouEmailData struct {
	baseEmailData
	ConsumerName     string
	OrganizationName string
	QuoteNumber      string
	HasAttachments   bool
}

type partnerOfferAcceptedEmailData struct {
	baseEmailData
	PartnerName string
	OfferID     string
}

type partnerOfferAcceptedConfirmationEmailData struct {
	baseEmailData
	PartnerName string
}

type partnerOfferRejectedEmailData struct {
	baseEmailData
	PartnerName string
	OfferID     string
	Reason      string
}

func renderEmailTemplate(name string, data any) (string, error) {
	templates := []string{"templates/base.html", "templates/" + name}
	tmpl, err := template.New("base.html").ParseFS(templateFS, templates...)
	if err != nil {
		return "", fmt.Errorf("parse email template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "email", data); err != nil {
		return "", fmt.Errorf("execute email template %s: %w", name, err)
	}
	return buf.String(), nil
}

func formatCurrencyEUR(cents int64) string {
	return fmt.Sprintf("€%.2f", float64(cents)/100)
}
