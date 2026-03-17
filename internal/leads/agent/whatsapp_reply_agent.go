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

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/orchestration"
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
	lead           *repository.Lead
	service        *repository.LeadService
	notes          []repository.LeadNote
	recentTimeline []repository.TimelineEvent
	analysis       *repository.AIAnalysis
	visitReport    *repository.AppointmentVisitReport
	photoAnalysis  *repository.PhotoAnalysis
	acceptedQuote  *ports.PublicQuoteSummary
	upcomingVisit  *ports.PublicAppointmentSummary
	pendingVisit   *ports.PublicAppointmentSummary
	requester      *ports.ReplyUserProfile
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

func (a *WhatsAppReplyAgent) SuggestWhatsAppReply(ctx context.Context, input ports.WhatsAppReplyInput) (ports.ReplySuggestionDraft, error) {
	replyContext, err := a.loadReplyContext(ctx, input)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}
	settings := a.loadOrganizationAISettings(ctx, input.OrganizationID)
	resolvedInput := input
	resolvedInput.Scenario = resolveEffectiveReplyScenario(
		input.Scenario,
		settings.WhatsAppDefaultReplyScenario,
		settings.QuoteRelatedReplyScenario,
		settings.AppointmentRelatedReplyScenario,
		replyContext.acceptedQuote != nil,
		replyContext.upcomingVisit != nil || replyContext.pendingVisit != nil,
	)
	r, sessionService, err := a.newRunner(settings.WhatsAppToneOfVoice)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}

	promptText := buildWhatsAppReplyPrompt(resolvedInput, replyContext, settings.WhatsAppToneOfVoice)
	sessionID := input.ConversationID.String()
	userID := "whatsapp-reply-conversation-" + input.ConversationID.String()
	if input.LeadID != nil && *input.LeadID != uuid.Nil {
		userID = "whatsapp-reply-" + input.LeadID.String()
	}
	outputText, err := runPromptTextSession(ctx, promptRunRequest{
		SessionService:       sessionService,
		Runner:               r,
		AppName:              a.appName,
		UserID:               userID,
		SessionID:            sessionID,
		CreateSessionMessage: "whatsapp reply: create session",
		RunFailureMessage:    "whatsapp reply: run failed",
		TraceLabel:           "whatsapp-reply",
	}, promptText)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}

	response := strings.TrimSpace(outputText)
	if response == "" {
		return ports.ReplySuggestionDraft{}, apperr.Internal("whatsapp reply: empty model response")
	}

	return ports.ReplySuggestionDraft{Text: response, EffectiveScenario: resolvedInput.Scenario}, nil
}

func (a *WhatsAppReplyAgent) newRunner(toneOfVoice string) (*runner.Runner, session.Service, error) {
	kimi := moonshot.NewModel(a.modelConfig)
	instruction, err := orchestration.BuildAgentInstruction("whatsapp-reply", whatsappReplySystemPrompt(toneOfVoice))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load whatsapp reply workspace context: %w", err)
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "WhatsAppReplyAgent",
		Model:       kimi,
		Description: "Suggests a single WhatsApp reply draft grounded in lead and conversation context.",
		Instruction: instruction,
	})
	if err != nil {
		return nil, nil, err
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        a.appName,
		SessionService: sessionService,
		Agent:          adkAgent,
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
	replyContext, err := a.loadLeadLinkedReplyContext(ctx, input)
	if err != nil {
		return whatsAppReplyContext{}, err
	}

	requester, err := a.loadRequester(ctx, input.RequesterUserID)
	if err != nil {
		return whatsAppReplyContext{}, err
	}
	replyContext.requester = requester
	if replyContext.lead != nil {
		var serviceID *uuid.UUID
		if replyContext.service != nil {
			serviceID = &replyContext.service.ID
		}
		replyContext.recentTimeline, err = loadRecentTimelineEvents(ctx, a.repo, replyContext.lead.ID, serviceID, input.OrganizationID)
		if err != nil {
			return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load timeline: %w", err)
		}
	}

	return replyContext, nil
}

func (a *WhatsAppReplyAgent) loadLeadLinkedReplyContext(ctx context.Context, input ports.WhatsAppReplyInput) (whatsAppReplyContext, error) {
	if input.LeadID == nil || *input.LeadID == uuid.Nil {
		return whatsAppReplyContext{}, nil
	}

	lead, err := a.repo.GetByID(ctx, *input.LeadID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load lead: %w", err)
	}
	service, err := a.repo.GetCurrentLeadService(ctx, *input.LeadID, input.OrganizationID)
	if err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: load current lead service: %w", err)
	}
	notes, err := a.repo.ListNotesByService(ctx, *input.LeadID, service.ID, input.OrganizationID)
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
	if err := attachAppointmentAssigneeNames(ctx, a.userReader, upcomingVisit, pendingVisit); err != nil {
		return whatsAppReplyContext{}, fmt.Errorf("whatsapp reply: enrich appointment assignee: %w", err)
	}

	return whatsAppReplyContext{
		lead:          &lead,
		service:       &service,
		notes:         notes,
		analysis:      analysis,
		visitReport:   visitReport,
		photoAnalysis: photoAnalysis,
		acceptedQuote: acceptedQuote,
		upcomingVisit: upcomingVisit,
		pendingVisit:  pendingVisit,
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
		if isIgnorableReplyUserLookupError(err) {
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

Selected reply scenario
%s

Conversation intent hints
%s

Communication style hints
%s

Requesting colleague
%s

Accepted quote overview
%s

Agenda overview
%s

Unknowns or missing context
%s

Recent timeline
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
- If a non-generic reply scenario is selected, treat that scenario as the primary drafting goal.
- Use the timing context to distinguish clearly between past and future appointments, deadlines, and updates.
- Use the intent hints and communication style hints as steering context when they fit the latest customer message.
- Use the real examples as style guidance when they fit, but adapt to the current customer and never copy them literally.
- Use older notes or analysis only when they clearly improve the answer.
- If the CRM context is incomplete, do not guess missing history, planning details, or accepted scope.
- Keep the message ready to paste into WhatsApp.
- If something important is missing, ask at most 2 concrete questions.
- Do not mention internal systems, AI analysis, or that this is a draft.
- Do not invent prices, promises, or technical facts that are not in the context.
- Output only the reply text.
`,
		formatWhatsAppReplyLeadID(replyContext.lead),
		formatWhatsAppReplyLeadName(replyContext.lead),
		formatWhatsAppReplyLeadAddress(replyContext.lead),
		formatWhatsAppReplyHousingContext(replyContext.lead),
		sanitizePromptField(input.DisplayName, 120),
		formatWhatsAppReplyServiceID(replyContext.service),
		formatWhatsAppReplyServiceType(replyContext.service),
		formatWhatsAppReplyServiceStatus(replyContext.service),
		formatWhatsAppReplyServicePipelineStage(replyContext.service),
		formatWhatsAppReplyConsumerNote(replyContext.service),
		formatWhatsAppReplyCustomerPreferences(replyContext.service),
		sanitizePromptField(toneOfVoice, 200),
		formatCurrentDateTimeBlock(),
		formatReplyScenarioBlock(input.Scenario, input.ScenarioNotes),
		formatWhatsAppIntentBlock(input),
		formatWhatsAppStyleSummary(input),
		formatRequesterBlock(replyContext.requester),
		formatQuoteOverviewBlock(replyContext.acceptedQuote),
		formatAgendaOverviewBlock(replyContext.upcomingVisit, replyContext.pendingVisit),
		formatUnknownsBlock(replyContext.service, replyContext.analysis, replyContext.acceptedQuote, replyContext.upcomingVisit, replyContext.pendingVisit, replyContext.lead != nil),
		formatTimelineBlock(replyContext.recentTimeline),
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
	return strings.TrimSpace(fmt.Sprintf(`## Tenant Tone Addendum

- Match this tenant tone of voice: %s.`, sanitizePromptField(toneOfVoice, 200)))
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

func formatWhatsAppReplyLeadID(lead *repository.Lead) string {
	if lead == nil || lead.ID == uuid.Nil {
		return valueNotProvided
	}
	return lead.ID.String()
}

func formatWhatsAppReplyLeadName(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadName(*lead)
}

func formatWhatsAppReplyLeadAddress(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadAddress(*lead)
}

func formatWhatsAppReplyHousingContext(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadHousingContext(*lead)
}

func formatWhatsAppReplyServiceID(service *repository.LeadService) string {
	if service == nil || service.ID == uuid.Nil {
		return valueNotProvided
	}
	return service.ID.String()
}

func formatWhatsAppReplyServiceType(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.ServiceType, 120)
}

func formatWhatsAppReplyServiceStatus(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.Status, 80)
}

func formatWhatsAppReplyServicePipelineStage(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.PipelineStage, 80)
}

func formatWhatsAppReplyConsumerNote(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return optionalPromptString(service.ConsumerNote, 800)
}

func formatWhatsAppReplyCustomerPreferences(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return formatJSONBlock(service.CustomerPreferences)
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
		fmt.Sprintf(freshnessLineFormat, formatFreshness(analysis.CreatedAt)),
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
	parts := []string{fmt.Sprintf(freshnessLineFormat, formatFreshness(report.CreatedAt))}
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
	parts := []string{fmt.Sprintf(freshnessLineFormat, formatFreshness(analysis.CreatedAt))}
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

	grouped := map[string][]string{
		noteGroupImportant:   {},
		noteGroupPreferences: {},
		noteGroupAccess:      {},
		noteGroupOther:       {},
	}
	start := 0
	if len(notes) > maxLeadNoteItems {
		start = len(notes) - maxLeadNoteItems
	}
	for _, note := range notes[start:] {
		body := sanitizePromptField(note.Body, 500)
		label := strings.ToLower(strings.TrimSpace(note.Type))
		entry := fmt.Sprintf("- %s [%s]", body, formatFreshness(note.CreatedAt))
		switch {
		case strings.Contains(label, "important"), strings.Contains(label, "urgent"), strings.Contains(label, "block"):
			grouped[noteGroupImportant] = append(grouped[noteGroupImportant], entry)
		case strings.Contains(label, "pref"), strings.Contains(label, "customer"):
			grouped[noteGroupPreferences] = append(grouped[noteGroupPreferences], entry)
		case strings.Contains(label, "access"), strings.Contains(label, "planning"), strings.Contains(label, "schedule"):
			grouped[noteGroupAccess] = append(grouped[noteGroupAccess], entry)
		default:
			grouped[noteGroupOther] = append(grouped[noteGroupOther], entry)
		}
	}
	sections := []string{}
	for _, heading := range []string{noteGroupImportant, noteGroupPreferences, noteGroupAccess, noteGroupOther} {
		if len(grouped[heading]) == 0 {
			continue
		}
		sections = append(sections, heading+"\n"+strings.Join(grouped[heading], "\n"))
	}
	if len(sections) == 0 {
		return valueNotProvided
	}
	return strings.Join(sections, "\n\n")
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

func formatQuoteOverviewBlock(quote *ports.PublicQuoteSummary) string {
	if quote == nil {
		return valueNotProvided
	}
	lines := []string{
		fmt.Sprintf("- Actualiteit: %s", formatFreshness(valueOrNow(quote.CreatedAt))),
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
	if len(quote.ScopeItems) > 0 {
		lines = append(lines, "- Scope-hoofdpunten: "+sanitizeUserInput(strings.Join(limitPromptList(quote.ScopeItems, maxQuoteScopeItems), "; "), 500))
	}
	if len(quote.LineItems) > 0 {
		itemLines := make([]string, 0, len(quote.LineItems))
		for _, item := range quote.LineItems {
			itemLines = append(itemLines, formatQuoteLineItem(item))
		}
		lines = append(lines, "- Inhoud:\n"+strings.Join(itemLines, "\n"))
	}
	return strings.Join(lines, "\n")
}

func formatAgendaOverviewBlock(upcomingVisit, pendingVisit *ports.PublicAppointmentSummary) string {
	if upcomingVisit == nil && pendingVisit == nil {
		return valueNotProvided
	}
	lines := []string{}
	if upcomingVisit != nil {
		lines = append(lines, formatAppointmentLine("Geplande afspraak", upcomingVisit))
	}
	if pendingVisit != nil {
		lines = append(lines, formatAppointmentLine("Open afspraakverzoek", pendingVisit))
	}
	return strings.Join(lines, "\n")
}

func valueOrNow(ts *time.Time) time.Time {
	if ts == nil {
		return replyLocalNow()
	}
	return *ts
}

func formatAppointmentLine(label string, appointment *ports.PublicAppointmentSummary) string {
	parts := []string{fmt.Sprintf("- %s: %s tot %s, %s (%s)", label, appointment.StartTime.Format(dateTimeLayout), appointment.EndTime.Format(dateTimeLayout), sanitizePromptField(appointment.Title, 120), sanitizePromptField(appointment.Status, 40))}
	parts = append(parts, fmt.Sprintf("  Timing: %s", formatFreshness(appointment.StartTime)))
	if appointment.Type != "" {
		parts = append(parts, "  Type: "+sanitizePromptField(appointment.Type, 80))
	}
	if appointment.AssignedUserName != nil {
		parts = append(parts, "  Ingepland bij: "+sanitizePromptField(*appointment.AssignedUserName, 120))
	}
	if appointment.Location != nil && strings.TrimSpace(*appointment.Location) != "" {
		parts = append(parts, "  Locatie: "+sanitizePromptField(*appointment.Location, 160))
	}
	if appointment.Description != nil && strings.TrimSpace(*appointment.Description) != "" {
		parts = append(parts, "  Omschrijving: "+sanitizePromptField(*appointment.Description, 220))
	}
	if appointment.MeetingLink != nil && strings.TrimSpace(*appointment.MeetingLink) != "" {
		parts = append(parts, "  Meeting link bekend")
	}
	return strings.Join(parts, "\n")
}

func formatCurrencyCents(totalCents int64) string {
	return fmt.Sprintf("EUR %.2f", float64(totalCents)/100)
}

func formatQuoteLineItem(item ports.PublicQuoteLineItemSummary) string {
	parts := []string{sanitizePromptField(item.Title, 100)}
	if item.Description != "" {
		parts = append(parts, sanitizePromptField(item.Description, 160))
	}
	if item.Quantity != "" {
		parts = append(parts, "aantal "+sanitizePromptField(item.Quantity, 32))
	}
	parts = append(parts, formatCurrencyCents(item.LineTotalCents))
	if item.IsOptional && !item.IsSelected {
		parts = append(parts, "optioneel niet geselecteerd")
	}
	return "  - " + strings.Join(parts, " | ")
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
