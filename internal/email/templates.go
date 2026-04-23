package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

// Global template cache. Initializing at the package level ensures O(1) rendering
// performance at runtime by avoiding repetitive filesystem I/O and parsing.
var (
	emailTemplates *template.Template
	parseOnce      sync.Once
	parseErr       error
)

// ─── Data Structures ─────────────────────────────────────────────────────────

// baseEmailData ordered for memory alignment (16-byte strings grouped).
type baseEmailData struct {
	Title      string
	Heading    string
	Subheading string
	CTALabel   string
	CTAURL     string
}

type (
	verificationEmailData       struct{ baseEmailData }
	passwordResetEmailData      struct{ baseEmailData }
	organizationInviteEmailData struct {
		baseEmailData
		OrganizationName string
	}
	partnerOfferAcceptedConfirmationEmailData struct {
		baseEmailData
		PartnerName    string
		HasAttachments bool
	}
)

type visitInviteEmailData struct {
	baseEmailData
	ConsumerName  string
	ScheduledDate string
	Address       string
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

type partnerOfferRejectedEmailData struct {
	baseEmailData
	PartnerName string
	OfferID     string
	Reason      string
}

// ─── Daily Digest Structures ─────────────────────────────────────────────────

type dailyDigestStaleLeadItem struct {
	ConsumerFirstName string
	ConsumerLastName  string
	ServiceType       string
	PipelineStage     string
	StaleReason       string
}

type dailyDigestAIActivity struct {
	GatekeeperRuns  int
	EstimatorRuns   int
	DispatcherRuns  int
	QuotesGenerated int
	PhotosAnalyzed  int
	OffersProcessed int
}

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
	StaleLeads       []dailyDigestStaleLeadItem
	AIActivity       dailyDigestAIActivity
	PipelineSnapshot dailyDigestPipelineSnapshot
	StaleLeadCount   int
}

// ─── Rendering Logic ────────────────────────────────────────────────────────

// getTemplates ensures templates are parsed exactly once (Thread-safe singleton).
func getTemplates() (*template.Template, error) {
	parseOnce.Do(func() {
		// Parsing all files in templates/ once. base.html must be included
		// to resolve the "email" define block.
		t := template.New("base.html")
		emailTemplates, parseErr = t.ParseFS(templateFS, "templates/*.html")
	})
	return emailTemplates, parseErr
}

// renderEmailTemplate executes a pre-parsed template.
// Complexity: O(1) parsing overhead + O(N) execution where N is data size.
func renderEmailTemplate(name string, data any) (string, error) {
	tmpl, err := getTemplates()
	if err != nil {
		return "", fmt.Errorf("templates unavailable: %w", err)
	}

	var buf bytes.Buffer
	// We execute the named template directly.
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("execute %s: %w", name, err)
	}
	return buf.String(), nil
}

// renderDailyDigestEmail maps domain input to template data.
func renderDailyDigestEmail(data DailyDigestInput) (string, error) {
	// O(N) allocation: Pre-allocating slice capacity to avoid re-allocations during loop.
	staleLeads := make([]dailyDigestStaleLeadItem, len(data.StaleLeads))
	for i, sl := range data.StaleLeads {
		staleLeads[i] = dailyDigestStaleLeadItem(sl)
	}

	tplData := dailyDigestEmailData{
		baseEmailData: baseEmailData{
			Title:    "Dagelijks overzicht",
			Heading:  "Goedemorgen ☀",
			CTALabel: "Bekijk dashboard",
			CTAURL:   data.DashboardURL,
		},
		OrganizationName: data.OrganizationName,
		Date:             data.Date,
		StaleLeads:       staleLeads,
		StaleLeadCount:   len(staleLeads),
		AIActivity: dailyDigestAIActivity{
			GatekeeperRuns:  data.GatekeeperRuns,
			EstimatorRuns:   data.EstimatorRuns,
			DispatcherRuns:  data.DispatcherRuns,
			QuotesGenerated: data.QuotesGenerated,
			PhotosAnalyzed:  data.PhotosAnalyzed,
			OffersProcessed: data.OffersProcessed,
		},
		PipelineSnapshot: dailyDigestPipelineSnapshot{
			Triage:             data.PipelineTriage,
			Nurturing:          data.PipelineNurturing,
			Estimation:         data.PipelineEstimation,
			Proposal:           data.PipelineProposal,
			Fulfillment:        data.PipelineFulfillment,
			ManualIntervention: data.PipelineManualIntervention,
		},
	}

	return renderEmailTemplate("daily_digest.html", tplData)
}

// formatCurrencyEUR handles float conversion for simple display.
func formatCurrencyEUR(cents int64) string {
	return fmt.Sprintf("€%.2f", float64(cents)/100)
}
