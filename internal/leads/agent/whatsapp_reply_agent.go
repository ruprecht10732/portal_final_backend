package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
	"portal_final_backend/platform/apperr"
)

const (
	whatsAppReplyAppName        = "whatsapp-reply-generator"
	maxWhatsAppExampleItems     = 4
	maxWhatsAppTranscriptItems  = 6
	maxWhatsAppMessageBodyChars = 750
	maxLeadNoteItems            = 5
)

type WhatsAppReplyAgent struct {
	repo                         repository.LeadsRepository
	modelConfig                  moonshot.Config
	appName                      string
	organizationAISettingsReader ports.OrganizationAISettingsReader
	quoteReader                  ports.ReplyQuoteReader
	appointmentViewer            ports.AppointmentPublicViewer
	userReader                   ports.ReplyUserReader
}

type whatsAppReplyContext struct {
	lead          repository.Lead
	service       repository.LeadService
	notes         []repository.LeadNote
	analysis      *repository.AIAnalysis
	visitReport   *repository.AppointmentVisitReport
	photoAnalysis *repository.PhotoAnalysis
	acceptedQuote *ports.PublicQuoteSummary
	upcomingVisit *ports.PublicAppointmentSummary
	pendingVisit  *ports.PublicAppointmentSummary
	requester     *ports.ReplyUserProfile
}

func NewWhatsAppReplyAgent(apiKey string, modelName string, repo repository.LeadsRepository) (*WhatsAppReplyAgent, error) {
	return &WhatsAppReplyAgent{
		repo:        repo,
		modelConfig: newMoonshotModelConfig(apiKey, modelName),
		appName:     whatsAppReplyAppName,
	}, nil
}

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader.
func (a *WhatsAppReplyAgent) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	a.organizationAISettingsReader = reader
}

func (a *WhatsAppReplyAgent) SetContextReaders(quoteReader ports.ReplyQuoteReader, appointmentViewer ports.AppointmentPublicViewer, userReader ports.ReplyUserReader) {
	a.quoteReader = quoteReader
	a.appointmentViewer = appointmentViewer
	a.userReader = userReader
}

func (a *WhatsAppReplyAgent) SuggestWhatsAppReply(ctx context.Context, input ports.WhatsAppReplyInput) (string, error) {
	replyContext, err := a.loadReplyContext(ctx, input)
	if err != nil {
		return "", err
	}
	settings := a.loadOrganizationAISettings(ctx, input.OrganizationID)
	r, sessionService, err := a.newRunner(settings.WhatsAppToneOfVoice)
	if err != nil {
		return "", err
	}

	promptText := buildWhatsAppReplyPrompt(input, replyContext, settings.WhatsAppToneOfVoice)
	sessionID := input.ConversationID.String()
	userID := "whatsapp-reply-" + input.LeadID.String()

	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   a.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("whatsapp reply: create session: %w", err)
	}
	defer func() {
		_ = sessionService.Delete(ctx, &session.DeleteRequest{
			AppName:   a.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
	}()

	userMessage := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: promptText}},
	}

	runConfig := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	var outputText strings.Builder
	if err := consumeRunEvents(r.Run(ctx, userID, sessionID, userMessage, runConfig), "whatsapp reply: run failed", func(event *session.Event) {
		if event.Content == nil {
			return
		}
		for _, part := range event.Content.Parts {
			outputText.WriteString(part.Text)
		}
	}); err != nil {
		return "", err
	}

	return strings.TrimSpace(outputText.String()), nil
}

func (a *WhatsAppReplyAgent) newRunner(toneOfVoice string) (*runner.Runner, session.Service, error) {
	kimi := moonshot.NewModel(a.modelConfig)
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "WhatsAppReplyAgent",
		Model:       kimi,
		Description: "Suggests a single WhatsApp reply draft grounded in lead and conversation context.",
		Instruction: whatsappReplySystemPrompt(toneOfVoice),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create whatsapp reply agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        a.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create whatsapp reply runner: %w", err)
	}

	return r, sessionService, nil
}

func (a *WhatsAppReplyAgent) loadOrganizationAISettings(ctx context.Context, organizationID uuid.UUID) ports.OrganizationAISettings {
	settings := ports.DefaultOrganizationAISettings()
	if a == nil || a.organizationAISettingsReader == nil {
		return settings
	}

	loadedSettings, err := a.organizationAISettingsReader(ctx, organizationID)
	if err != nil {
		log.Printf("whatsapp reply: failed to load organization AI settings for %s: %v", organizationID, err)
		return settings
	}
	if strings.TrimSpace(loadedSettings.WhatsAppToneOfVoice) == "" {
		loadedSettings.WhatsAppToneOfVoice = settings.WhatsAppToneOfVoice
	}
	return loadedSettings
}

func (a *WhatsAppReplyAgent) loadReplyContext(ctx context.Context, input ports.WhatsAppReplyInput) (whatsAppReplyContext, error) {
	lead, err := a.repo.GetByID(ctx, input.LeadID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load lead: %w", err)
	}

	service, err := a.repo.GetCurrentLeadService(ctx, input.LeadID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load current lead service: %w", err)
	}

	notes, err := a.repo.ListNotesByService(ctx, input.LeadID, service.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load notes: %w", err)
	}

	analysis, err := a.loadLatestAIAnalysis(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	visitReport, err := a.loadLatestVisitReport(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	photoAnalysis, err := a.loadLatestPhotoAnalysis(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	acceptedQuote, err := a.loadAcceptedQuote(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	upcomingVisit, pendingVisit, err := a.loadAgenda(ctx, lead.ID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	requester, err := a.loadRequester(ctx, input.RequesterUserID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}

	return whatsAppReplyContext{
		lead:          lead,
		service:       service,
		notes:         notes,
		analysis:      analysis,
		visitReport:   visitReport,
		photoAnalysis: photoAnalysis,
		acceptedQuote: acceptedQuote,
		upcomingVisit: upcomingVisit,
		pendingVisit:  pendingVisit,
		requester:     requester,
	}, nil
}

func (a *WhatsAppReplyAgent) loadAcceptedQuote(ctx context.Context, serviceID, organizationID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	if a == nil || a.quoteReader == nil {
		return nil, nil
	}
	quote, err := a.quoteReader.GetAcceptedQuote(ctx, serviceID, organizationID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("whatsapp reply: load accepted quote: %w", err)
	}
	return quote, nil
}

func (a *WhatsAppReplyAgent) loadAgenda(ctx context.Context, leadID, organizationID uuid.UUID) (*ports.PublicAppointmentSummary, *ports.PublicAppointmentSummary, error) {
	if a == nil || a.appointmentViewer == nil {
		return nil, nil, nil
	}
	upcomingVisit, err := a.appointmentViewer.GetUpcomingVisit(ctx, leadID, organizationID)
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		return nil, nil, fmt.Errorf("whatsapp reply: load upcoming visit: %w", err)
	}
	pendingVisit, err := a.appointmentViewer.GetPendingVisit(ctx, leadID, organizationID)
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		return nil, nil, fmt.Errorf("whatsapp reply: load pending visit: %w", err)
	}
	return upcomingVisit, pendingVisit, nil
}

func (a *WhatsAppReplyAgent) loadRequester(ctx context.Context, requesterUserID uuid.UUID) (*ports.ReplyUserProfile, error) {
	if a == nil || a.userReader == nil || requesterUserID == uuid.Nil {
		return nil, nil
	}
	requester, err := a.userReader.GetUserProfile(ctx, requesterUserID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("whatsapp reply: load requester: %w", err)
	}
	return requester, nil
}

func (a *WhatsAppReplyAgent) loadLatestAIAnalysis(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.AIAnalysis, error) {
	analysis, err := a.repo.GetLatestAIAnalysis(ctx, serviceID, organizationID)
	if err == nil {
		return &analysis, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("whatsapp reply: load ai analysis: %w", err)
}

func (a *WhatsAppReplyAgent) loadLatestVisitReport(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.AppointmentVisitReport, error) {
	visitReport, err := a.repo.GetLatestAppointmentVisitReportByService(ctx, serviceID, organizationID)
	if err == nil {
		return visitReport, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("whatsapp reply: load visit report: %w", err)
}

func (a *WhatsAppReplyAgent) loadLatestPhotoAnalysis(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.PhotoAnalysis, error) {
	photoAnalysis, err := a.repo.GetLatestPhotoAnalysis(ctx, serviceID, organizationID)
	if err == nil {
		return &photoAnalysis, nil
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrPhotoAnalysisNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("whatsapp reply: load photo analysis: %w", err)
}

func buildWhatsAppReplyPrompt(
	input ports.WhatsAppReplyInput,
	replyContext whatsAppReplyContext,
	toneOfVoice string,
) string {
	return fmt.Sprintf(`Lead context
- Lead ID: %s
- Naam: %s
- Adres: %s
- Wooncontext: %s
- WhatsApp display name: %s

Service context
- Service ID: %s
- Service type: %s
- Status: %s
- Pipeline stage: %s
- Consumer note: %s
- Preferences: %s

Reply style
- Tone of voice: %s

Current date and time
%s

Requesting colleague
%s

Accepted quote overview
%s

Agenda overview
%s

Latest AI analysis
%s

Latest visit report
%s

Latest photo analysis
%s

Lead notes
%s

Recent human corrections from this tenant
%s

Recent real examples from this tenant
%s

Recent WhatsApp transcript
%s

Task
- Draft one WhatsApp reply in Dutch for the company to send to this customer.
- Prioritize the latest customer message and the most recent conversation turns.
- Use the real examples as style guidance when they fit, but adapt to the current customer and never copy them literally.
- Use older notes or analysis only when they clearly improve the answer.
- Keep the message ready to paste into WhatsApp.
- If something important is missing, ask at most 2 concrete questions.
- Do not mention internal systems, AI analysis, or that this is a draft.
- Do not invent prices, promises, or technical facts that are not in the context.
- Output only the reply text.
`,
		replyContext.lead.ID,
		buildLeadName(replyContext.lead),
		buildLeadAddress(replyContext.lead),
		buildLeadHousingContext(replyContext.lead),
		sanitizePromptField(input.DisplayName, 120),
		replyContext.service.ID,
		sanitizePromptField(replyContext.service.ServiceType, 120),
		sanitizePromptField(replyContext.service.Status, 80),
		sanitizePromptField(replyContext.service.PipelineStage, 80),
		optionalPromptString(replyContext.service.ConsumerNote, 800),
		formatJSONBlock(replyContext.service.CustomerPreferences),
		sanitizePromptField(toneOfVoice, 200),
		formatCurrentDateTimeBlock(),
		formatRequesterBlock(replyContext.requester),
		formatQuoteOverviewBlock(replyContext.acceptedQuote),
		formatAgendaOverviewBlock(replyContext.upcomingVisit, replyContext.pendingVisit),
		formatAIAnalysisBlock(replyContext.analysis),
		formatVisitReportBlock(replyContext.visitReport),
		formatPhotoAnalysisBlock(replyContext.photoAnalysis),
		formatLeadNotesBlock(replyContext.notes),
		formatWhatsAppFeedbackMemory(input.Feedback),
		formatWhatsAppExamples(input.Examples),
		formatWhatsAppTranscript(input.Messages),
	)
}

func whatsappReplySystemPrompt(toneOfVoice string) string {
	if strings.TrimSpace(toneOfVoice) == "" {
		toneOfVoice = ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	}
	return strings.TrimSpace(fmt.Sprintf(`You write customer-ready WhatsApp replies for a Dutch home-services company.

Rules:
- Return exactly one draft reply in Dutch.
- Keep it concise and customer-ready.
- Match this tenant tone of voice: %s.
- Prefer short paragraphs suitable for WhatsApp.
- Ground the reply in the provided lead, service, and conversation context.
- Prioritize the latest inbound message and the most recent conversation turns over older notes.
- If the latest customer message asks a direct question, answer it directly when the context supports it.
- If details are still needed, ask at most two clear questions and explain briefly why.
- Never expose internal reasoning, raw analysis data, or uncertainty labels.
- Never fabricate pricing, availability, measurements, or policy details.
- Output only the message text, with no title or surrounding quotes.`, sanitizePromptField(toneOfVoice, 200)))
}

func formatWhatsAppFeedbackMemory(items []ports.WhatsAppReplyFeedback) string {
	if len(items) == 0 {
		return valueNotProvided
	}

	var sb strings.Builder
	count := 0
	for _, item := range items {
		if count >= maxWhatsAppExampleItems {
			break
		}
		aiReply := sanitizePromptField(item.AIReply, maxWhatsAppMessageBodyChars)
		humanReply := sanitizePromptField(item.HumanReply, maxWhatsAppMessageBodyChars)
		if aiReply == valueNotProvided || humanReply == valueNotProvided {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("AI-draft: ")
		sb.WriteString(aiReply)
		sb.WriteString("\nMenselijke correctie: ")
		sb.WriteString(humanReply)
		count++
	}
	if sb.Len() == 0 {
		return valueNotProvided
	}
	return sb.String()
}

func buildLeadName(lead repository.Lead) string {
	name := strings.TrimSpace(strings.TrimSpace(lead.ConsumerFirstName) + " " + strings.TrimSpace(lead.ConsumerLastName))
	if name == "" {
		return valueNotProvided
	}
	return sanitizePromptField(name, 120)
}

func buildLeadAddress(lead repository.Lead) string {
	return joinPromptFields(
		sanitizePromptField(lead.AddressStreet, 120),
		sanitizePromptField(lead.AddressHouseNumber, 24),
		sanitizePromptField(lead.AddressZipCode, 24),
		sanitizePromptField(lead.AddressCity, 80),
	)
}

func buildLeadHousingContext(lead repository.Lead) string {
	parts := make([]string, 0, 3)
	if lead.EnergyBouwjaar != nil {
		parts = append(parts, fmt.Sprintf("bouwjaar %d", *lead.EnergyBouwjaar))
	}
	if lead.EnergyClass != nil && strings.TrimSpace(*lead.EnergyClass) != "" {
		parts = append(parts, "energielabel "+sanitizePromptField(*lead.EnergyClass, 16))
	}
	if lead.EnergyGebouwtype != nil && strings.TrimSpace(*lead.EnergyGebouwtype) != "" {
		parts = append(parts, sanitizePromptField(*lead.EnergyGebouwtype, 80))
	}
	if len(parts) == 0 {
		return valueNotProvided
	}
	return strings.Join(parts, ", ")
}

func optionalPromptString(value *string, maxLen int) string {
	if value == nil {
		return valueNotProvided
	}
	return sanitizePromptField(*value, maxLen)
}

func formatJSONBlock(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" || trimmed == "[]" {
		return valueNotProvided
	}
	return sanitizeUserInput(trimmed, 1200)
}

func formatAIAnalysisBlock(analysis *repository.AIAnalysis) string {
	if analysis == nil {
		return valueNotProvided
	}

	parts := []string{
		fmt.Sprintf("- Samenvatting: %s", sanitizePromptField(analysis.Summary, 500)),
		fmt.Sprintf("- Aanbevolen actie: %s", sanitizePromptField(analysis.RecommendedAction, 80)),
	}
	if len(analysis.MissingInformation) > 0 {
		parts = append(parts, fmt.Sprintf("- Actief ontbrekend: %s", sanitizeUserInput(strings.Join(limitPromptList(analysis.MissingInformation, 3), "; "), 300)))
	}
	if len(analysis.ResolvedInformation) > 0 {
		parts = append(parts, fmt.Sprintf("- Reeds bevestigd: %s", sanitizeUserInput(strings.Join(limitPromptList(analysis.ResolvedInformation, 3), "; "), 300)))
	}
	return strings.Join(parts, "\n")
}

func formatVisitReportBlock(report *repository.AppointmentVisitReport) string {
	if report == nil {
		return valueNotProvided
	}
	parts := make([]string, 0, 3)
	if report.Measurements != nil && strings.TrimSpace(*report.Measurements) != "" {
		parts = append(parts, "- Metingen: "+sanitizePromptField(*report.Measurements, 800))
	}
	if report.AccessDifficulty != nil && strings.TrimSpace(*report.AccessDifficulty) != "" {
		parts = append(parts, "- Bereikbaarheid: "+sanitizePromptField(*report.AccessDifficulty, 400))
	}
	if report.Notes != nil && strings.TrimSpace(*report.Notes) != "" {
		parts = append(parts, "- Notities: "+sanitizePromptField(*report.Notes, 800))
	}
	if len(parts) == 0 {
		return valueNotProvided
	}
	return strings.Join(parts, "\n")
}

func formatPhotoAnalysisBlock(analysis *repository.PhotoAnalysis) string {
	if analysis == nil {
		return valueNotProvided
	}
	parts := []string{}
	if summary := strings.TrimSpace(analysis.Summary); summary != "" {
		parts = append(parts, "- Samenvatting: "+sanitizePromptField(summary, 800))
	}
	if scope := strings.TrimSpace(analysis.ScopeAssessment); scope != "" {
		parts = append(parts, "- Scope: "+sanitizePromptField(scope, 500))
	}
	if costs := strings.TrimSpace(analysis.CostIndicators); costs != "" {
		parts = append(parts, "- Kostenindicaties: "+sanitizePromptField(costs, 500))
	}
	if len(analysis.Discrepancies) > 0 {
		parts = append(parts, "- Afwijkingen: "+sanitizeUserInput(strings.Join(analysis.Discrepancies, "; "), 600))
	}
	if len(analysis.NeedsOnsiteMeasurement) > 0 {
		parts = append(parts, "- Nog op locatie meten: "+sanitizeUserInput(strings.Join(analysis.NeedsOnsiteMeasurement, "; "), 600))
	}
	if len(parts) == 0 {
		return valueNotProvided
	}
	return strings.Join(parts, "\n")
}

func formatLeadNotesBlock(notes []repository.LeadNote) string {
	if len(notes) == 0 {
		return valueNotProvided
	}

	start := 0
	if len(notes) > maxLeadNoteItems {
		start = len(notes) - maxLeadNoteItems
	}
	lines := make([]string, 0, len(notes)-start)
	for _, note := range notes[start:] {
		body := sanitizePromptField(note.Body, 500)
		label := sanitizePromptField(note.Type, 40)
		if label == valueNotProvided {
			label = "notitie"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s (%s)", label, body, note.CreatedAt.Format(dateTimeLayout)))
	}
	return strings.Join(lines, "\n")
}

func formatRequesterBlock(requester *ports.ReplyUserProfile) string {
	if requester == nil {
		return valueNotProvided
	}
	name := joinPromptFields(
		optionalPromptString(requester.FirstName, 80),
		optionalPromptString(requester.LastName, 80),
	)
	lines := []string{}
	if name != valueNotProvided {
		lines = append(lines, "- Naam: "+name)
	}
	if email := sanitizePromptField(requester.Email, 160); email != valueNotProvided {
		lines = append(lines, "- E-mailadres: "+email)
	}
	if len(lines) == 0 {
		return valueNotProvided
	}
	return strings.Join(lines, "\n")
}

func formatCurrentDateTimeBlock() string {
	now := time.Now()
	return strings.Join([]string{
		"- Nu: " + now.Format(time.RFC3339),
		"- Tijdzone: " + now.Format("MST"),
		"- Gebruik dit om te bepalen of afspraken, deadlines en gebeurtenissen in het verleden of de toekomst liggen.",
	}, "\n")
}

func formatQuoteOverviewBlock(quote *ports.PublicQuoteSummary) string {
	if quote == nil {
		return valueNotProvided
	}
	lines := []string{
		"- Offertenummer: " + sanitizePromptField(quote.QuoteNumber, 80),
		"- Status: " + sanitizePromptField(quote.Status, 40),
		fmt.Sprintf("- Totaal: %s", formatCurrencyCents(quote.TotalCents)),
	}
	if quote.AcceptedAt != nil {
		lines = append(lines, "- Geaccepteerd op: "+quote.AcceptedAt.Format(dateTimeLayout))
	}
	if quote.ValidUntil != nil {
		lines = append(lines, "- Geldig tot: "+quote.ValidUntil.Format(dateTimeLayout))
	}
	if quote.Notes != nil && strings.TrimSpace(*quote.Notes) != "" {
		lines = append(lines, "- Offertenotities: "+sanitizePromptField(*quote.Notes, 400))
	}
	return strings.Join(lines, "\n")
}

func formatAgendaOverviewBlock(upcomingVisit, pendingVisit *ports.PublicAppointmentSummary) string {
	if upcomingVisit == nil && pendingVisit == nil {
		return valueNotProvided
	}
	lines := []string{}
	if upcomingVisit != nil {
		lines = append(lines, fmt.Sprintf("- Geplande afspraak: %s tot %s, %s (%s)", upcomingVisit.StartTime.Format(dateTimeLayout), upcomingVisit.EndTime.Format(dateTimeLayout), sanitizePromptField(upcomingVisit.Title, 120), sanitizePromptField(upcomingVisit.Status, 40)))
	}
	if pendingVisit != nil {
		lines = append(lines, fmt.Sprintf("- Open afspraakverzoek: %s tot %s, %s (%s)", pendingVisit.StartTime.Format(dateTimeLayout), pendingVisit.EndTime.Format(dateTimeLayout), sanitizePromptField(pendingVisit.Title, 120), sanitizePromptField(pendingVisit.Status, 40)))
	}
	return strings.Join(lines, "\n")
}

func formatCurrencyCents(totalCents int64) string {
	return fmt.Sprintf("EUR %.2f", float64(totalCents)/100)
}

func formatWhatsAppTranscript(messages []ports.WhatsAppReplyMessage) string {
	if len(messages) == 0 {
		return valueNotProvided
	}
	start := 0
	if len(messages) > maxWhatsAppTranscriptItems {
		start = len(messages) - maxWhatsAppTranscriptItems
	}
	lines := make([]string, 0, len(messages)-start)
	for _, message := range messages[start:] {
		role := "Klant"
		if strings.EqualFold(strings.TrimSpace(message.Direction), "outbound") {
			role = "Bedrijf"
		}
		body := sanitizePromptField(message.Body, maxWhatsAppMessageBodyChars)
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", message.CreatedAt.Format(dateTimeLayout), role, body))
	}
	return strings.Join(lines, "\n")
}

func formatWhatsAppExamples(examples []ports.WhatsAppReplyExample) string {
	if len(examples) == 0 {
		return valueNotProvided
	}
	limit := len(examples)
	if limit > maxWhatsAppExampleItems {
		limit = maxWhatsAppExampleItems
	}
	lines := make([]string, 0, limit*3)
	for _, example := range examples[:limit] {
		lines = append(lines,
			fmt.Sprintf("- [%s] Klant: %s", example.CreatedAt.Format(dateTimeLayout), sanitizePromptField(example.CustomerMessage, 280)),
			fmt.Sprintf("  Bedrijf: %s", sanitizePromptField(example.Reply, 280)),
		)
	}
	return strings.Join(lines, "\n")
}

func limitPromptList(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
