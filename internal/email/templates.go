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
	PartnerName    string
	HasAttachments bool
}

type partnerOfferRejectedEmailData struct {
	baseEmailData
	PartnerName string
	OfferID     string
	Reason      string
}

// dailyDigestStaleLeadItem maps stale leads for the digest template.
type dailyDigestStaleLeadItem struct {
	ConsumerFirstName string
	ConsumerLastName  string
	ServiceType       string
	PipelineStage     string
	StaleReason       string
}

// dailyDigestAIActivity maps AI activity counts for the digest template.
type dailyDigestAIActivity struct {
	GatekeeperRuns  int
	EstimatorRuns   int
	DispatcherRuns  int
	QuotesGenerated int
	PhotosAnalyzed  int
	OffersProcessed int
}

// dailyDigestPipelineSnapshot maps pipeline counts for the digest template.
type dailyDigestPipelineSnapshot struct {
	Triage             int
	Nurturing          int
	Estimation         int
	Proposal           int
	Fulfillment        int
	ManualIntervention int
}

type dailyDigestEmailData struct {
	baseEmailData
	OrganizationName string
	Date             string
	AIActivity       dailyDigestAIActivity
	StaleLeads       []dailyDigestStaleLeadItem
	StaleLeadCount   int
	PipelineSnapshot dailyDigestPipelineSnapshot
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

func renderDailyDigestEmail(data DailyDigestInput) (string, error) {
	staleLeads := make([]dailyDigestStaleLeadItem, len(data.StaleLeads))
	for i, sl := range data.StaleLeads {
		staleLeads[i] = dailyDigestStaleLeadItem(sl)
	}

	templateData := dailyDigestEmailData{
		baseEmailData: baseEmailData{
			Title:    "Dagelijks overzicht",
			Heading:  "Goedemorgen ☀",
			CTALabel: "Bekijk dashboard",
			CTAURL:   data.DashboardURL,
		},
		OrganizationName: data.OrganizationName,
		Date:             data.Date,
		AIActivity: dailyDigestAIActivity{
			GatekeeperRuns:  data.GatekeeperRuns,
			EstimatorRuns:   data.EstimatorRuns,
			DispatcherRuns:  data.DispatcherRuns,
			QuotesGenerated: data.QuotesGenerated,
			PhotosAnalyzed:  data.PhotosAnalyzed,
			OffersProcessed: data.OffersProcessed,
		},
		StaleLeads:     staleLeads,
		StaleLeadCount: len(staleLeads),
		PipelineSnapshot: dailyDigestPipelineSnapshot{
			Triage:             data.PipelineTriage,
			Nurturing:          data.PipelineNurturing,
			Estimation:         data.PipelineEstimation,
			Proposal:           data.PipelineProposal,
			Fulfillment:        data.PipelineFulfillment,
			ManualIntervention: data.PipelineManualIntervention,
		},
	}

	return renderEmailTemplate("daily_digest.html", templateData)
}
