package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/orchestration"
	"portal_final_backend/platform/ai/moonshot"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	emailReplyAppName         = "email-reply-generator"
	maxEmailExampleItems      = 4
	maxEmailBodyChars         = 1600
	maxEmailFeedbackBodyChars = 900
)

type EmailReplyAgent struct {
	repo                         repository.LeadsRepository
	modelConfig                  moonshot.Config
	appName                      string
	organizationAISettingsReader ports.OrganizationAISettingsReader
	quoteReader                  ports.ReplyQuoteReader
	appointmentViewer            ports.AppointmentPublicViewer
	userReader                   ports.ReplyUserReader
}

type emailReplyLookupStore interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.Lead, error)
	GetByPhoneOrEmail(ctx context.Context, phone string, email string, organizationID uuid.UUID) (*repository.LeadSummary, []repository.LeadService, error)
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (repository.LeadService, error)
	GetCurrentLeadService(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (repository.LeadService, error)
}

type emailReplyContext struct {
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

func NewEmailReplyAgent(apiKey string, modelName string, repo repository.LeadsRepository) (*EmailReplyAgent, error) {
	return &EmailReplyAgent{
		repo:        repo,
		modelConfig: newMoonshotModelConfig(apiKey, modelName),
		appName:     emailReplyAppName,
	}, nil
}

func (a *EmailReplyAgent) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	a.organizationAISettingsReader = reader
}

func (a *EmailReplyAgent) SetContextReaders(quoteReader ports.ReplyQuoteReader, appointmentViewer ports.AppointmentPublicViewer, userReader ports.ReplyUserReader) {
	a.quoteReader = quoteReader
	a.appointmentViewer = appointmentViewer
	a.userReader = userReader
}

func (a *EmailReplyAgent) SuggestEmailReply(ctx context.Context, input ports.EmailReplyInput) (ports.ReplySuggestionDraft, error) {
	replyContext, err := a.loadReplyContext(ctx, input)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}
	settings := a.loadOrganizationAISettings(ctx, input.OrganizationID)
	resolvedInput := input
	resolvedInput.Scenario = resolveEffectiveReplyScenario(
		input.Scenario,
		settings.EmailDefaultReplyScenario,
		settings.QuoteRelatedReplyScenario,
		settings.AppointmentRelatedReplyScenario,
		replyContext.acceptedQuote != nil,
		replyContext.upcomingVisit != nil || replyContext.pendingVisit != nil,
	)
	r, sessionService, err := a.newRunner(settings.WhatsAppToneOfVoice)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}

	promptText := buildEmailReplyPrompt(resolvedInput, replyContext, settings.WhatsAppToneOfVoice)
	sessionID := uuid.NewString()
	userID := "email-reply-" + input.OrganizationID.String() + ":" + sanitizeUserInput(strings.ToLower(strings.TrimSpace(input.CustomerEmail)), 120)
	outputText, err := runPromptTextSession(ctx, promptRunRequest{
		SessionService:       sessionService,
		Runner:               r,
		AppName:              a.appName,
		UserID:               userID,
		SessionID:            sessionID,
		CreateSessionMessage: "email reply: create session",
		RunFailureMessage:    "email reply: run failed",
		TraceLabel:           "email-reply",
	}, promptText)
	if err != nil {
		return ports.ReplySuggestionDraft{}, err
	}

	response := strings.TrimSpace(outputText)
	if response == "" {
		return ports.ReplySuggestionDraft{}, apperr.Internal("email reply: empty model response")
	}

	return ports.ReplySuggestionDraft{Text: response, EffectiveScenario: resolvedInput.Scenario}, nil
}

func (a *EmailReplyAgent) newRunner(toneOfVoice string) (*runner.Runner, session.Service, error) {
	kimi := moonshot.NewModel(a.modelConfig)
	instruction, err := orchestration.BuildAgentInstruction("email-reply", emailReplySystemPrompt(toneOfVoice))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load email reply workspace context: %w", err)
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "EmailReplyAgent",
		Model:       kimi,
		Description: "Suggests a single email reply draft grounded in lead and service context.",
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
		return nil, nil, fmt.Errorf("failed to create email reply runner: %w", err)
	}

	return r, sessionService, nil
}

func (a *EmailReplyAgent) loadOrganizationAISettings(ctx context.Context, organizationID uuid.UUID) ports.OrganizationAISettings {
	settings := ports.DefaultOrganizationAISettings()
	if a == nil || a.organizationAISettingsReader == nil {
		return settings
	}

	loadedSettings, err := a.organizationAISettingsReader(ctx, organizationID)
	if err != nil {
		log.Printf("email reply: failed to load organization AI settings for %s: %v", organizationID, err)
		return settings
	}
	if strings.TrimSpace(loadedSettings.WhatsAppToneOfVoice) == "" {
		loadedSettings.WhatsAppToneOfVoice = settings.WhatsAppToneOfVoice
	}
	return loadedSettings
}

func (a *EmailReplyAgent) loadReplyContext(ctx context.Context, input ports.EmailReplyInput) (emailReplyContext, error) {
	lead, service, err := resolveEmailReplyLeadAndService(ctx, a.repo, input)
	if err != nil {
		return emailReplyContext{}, err
	}

	contextData := emailReplyContext{
		lead:    lead,
		service: service,
		notes:   nil,
	}
	if err := a.applySharedReplyContext(ctx, input, &contextData); err != nil {
		return emailReplyContext{}, err
	}

	if service == nil {
		if err := a.loadLeadOnlyNotes(ctx, input.OrganizationID, &contextData); err != nil {
			return emailReplyContext{}, err
		}
		return contextData, nil
	}

	if err := a.loadServiceReplyContext(ctx, input.OrganizationID, &contextData); err != nil {
		return emailReplyContext{}, err
	}
	return contextData, nil
}

func resolveEmailReplyLeadAndService(ctx context.Context, store emailReplyLookupStore, input ports.EmailReplyInput) (*repository.Lead, *repository.LeadService, error) {
	if store == nil {
		return nil, nil, nil
	}

	service, err := loadEmailReplyServiceByID(ctx, store, input)
	if err != nil {
		return nil, nil, err
	}

	lead, err := loadEmailReplyLeadByIDOrEmail(ctx, store, input)
	if err != nil {
		return nil, nil, err
	}

	lead, err = hydrateEmailReplyLeadFromService(ctx, store, input.OrganizationID, lead, service)
	if err != nil {
		return nil, nil, err
	}

	if service == nil && lead != nil && lead.ID != uuid.Nil {
		service, err = loadEmailReplyCurrentService(ctx, store, lead.ID, input.OrganizationID)
		if err != nil {
			return nil, nil, err
		}
	}

	return lead, service, nil
}

func loadEmailReplyServiceByID(ctx context.Context, store emailReplyLookupStore, input ports.EmailReplyInput) (*repository.LeadService, error) {
	if input.LeadServiceID == nil || *input.LeadServiceID == uuid.Nil {
		return nil, nil
	}
	service, err := store.GetLeadServiceByID(ctx, *input.LeadServiceID, input.OrganizationID)
	if err == nil {
		return &service, nil
	}
	if errors.Is(err, repository.ErrServiceNotFound) || errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("email reply: load lead service: %w", err)
}

func loadEmailReplyLeadByIDOrEmail(ctx context.Context, store emailReplyLookupStore, input ports.EmailReplyInput) (*repository.Lead, error) {
	if input.LeadID != nil && *input.LeadID != uuid.Nil {
		lead, err := store.GetByID(ctx, *input.LeadID, input.OrganizationID)
		if err == nil {
			return &lead, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("email reply: load lead: %w", err)
		}
	}

	if strings.TrimSpace(input.CustomerEmail) == "" {
		return nil, nil
	}

	summary, _, err := store.GetByPhoneOrEmail(ctx, "", input.CustomerEmail, input.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("email reply: load lead by email: %w", err)
	}
	if summary == nil || summary.ID == uuid.Nil {
		return nil, nil
	}

	lead, err := store.GetByID(ctx, summary.ID, input.OrganizationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load lead: %w", err)
	}
	return &lead, nil
}

func hydrateEmailReplyLeadFromService(ctx context.Context, store emailReplyLookupStore, organizationID uuid.UUID, lead *repository.Lead, service *repository.LeadService) (*repository.Lead, error) {
	if lead != nil || service == nil || service.LeadID == uuid.Nil {
		return lead, nil
	}

	loadedLead, err := store.GetByID(ctx, service.LeadID, organizationID)
	if err == nil {
		return &loadedLead, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("email reply: load lead from service: %w", err)
}

func loadEmailReplyCurrentService(ctx context.Context, store emailReplyLookupStore, leadID, organizationID uuid.UUID) (*repository.LeadService, error) {
	service, err := store.GetCurrentLeadService(ctx, leadID, organizationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrServiceNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load current lead service: %w", err)
	}
	return &service, nil
}

func (a *EmailReplyAgent) applySharedReplyContext(ctx context.Context, input ports.EmailReplyInput, contextData *emailReplyContext) error {
	requester, err := a.loadRequester(ctx, input.RequesterUserID)
	if err != nil {
		return err
	}
	contextData.requester = requester

	if contextData.lead == nil {
		return nil
	}

	upcomingVisit, pendingVisit, err := a.loadAgenda(ctx, contextData.lead.ID, input.OrganizationID)
	if err != nil {
		return err
	}
	if err := attachAppointmentAssigneeNames(ctx, a.userReader, upcomingVisit, pendingVisit); err != nil {
		return fmt.Errorf("email reply: enrich appointment assignee: %w", err)
	}
	contextData.upcomingVisit = upcomingVisit
	contextData.pendingVisit = pendingVisit

	var serviceID *uuid.UUID
	if contextData.service != nil {
		serviceID = &contextData.service.ID
	}
	recentTimeline, err := loadRecentTimelineEvents(ctx, a.repo, contextData.lead.ID, serviceID, input.OrganizationID)
	if err != nil {
		return fmt.Errorf("email reply: load timeline: %w", err)
	}
	contextData.recentTimeline = recentTimeline
	return nil
}

func (a *EmailReplyAgent) loadLeadOnlyNotes(ctx context.Context, organizationID uuid.UUID, contextData *emailReplyContext) error {
	if contextData.lead == nil {
		return nil
	}
	notes, err := a.repo.ListLeadNotes(ctx, contextData.lead.ID, organizationID)
	if err != nil {
		return fmt.Errorf("email reply: load lead notes: %w", err)
	}
	contextData.notes = notes
	return nil
}

func (a *EmailReplyAgent) loadServiceReplyContext(ctx context.Context, organizationID uuid.UUID, contextData *emailReplyContext) error {
	service := contextData.service
	if service == nil {
		return nil
	}

	leadID := service.LeadID
	if contextData.lead != nil && contextData.lead.ID != uuid.Nil {
		leadID = contextData.lead.ID
	}

	notes, err := a.repo.ListNotesByService(ctx, leadID, service.ID, organizationID)
	if err != nil {
		return fmt.Errorf("email reply: load notes: %w", err)
	}
	contextData.notes = notes

	acceptedQuote, err := a.loadAcceptedQuote(ctx, service.ID, organizationID)
	if err != nil {
		return err
	}
	contextData.acceptedQuote = acceptedQuote

	analysis, err := a.loadLatestAIAnalysis(ctx, service.ID, organizationID)
	if err != nil {
		return err
	}
	visitReport, err := a.loadLatestVisitReport(ctx, service.ID, organizationID)
	if err != nil {
		return err
	}
	photoAnalysis, err := a.loadLatestPhotoAnalysis(ctx, service.ID, organizationID)
	if err != nil {
		return err
	}

	contextData.analysis = analysis
	contextData.visitReport = visitReport
	contextData.photoAnalysis = photoAnalysis
	return nil
}

func (a *EmailReplyAgent) loadAcceptedQuote(ctx context.Context, serviceID, organizationID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	if a == nil || a.quoteReader == nil {
		return nil, nil
	}
	quote, err := a.quoteReader.GetAcceptedQuote(ctx, serviceID, organizationID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load accepted quote: %w", err)
	}
	return quote, nil
}

func (a *EmailReplyAgent) loadAgenda(ctx context.Context, leadID, organizationID uuid.UUID) (*ports.PublicAppointmentSummary, *ports.PublicAppointmentSummary, error) {
	if a == nil || a.appointmentViewer == nil {
		return nil, nil, nil
	}
	upcomingVisit, err := a.appointmentViewer.GetUpcomingVisit(ctx, leadID, organizationID)
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		return nil, nil, fmt.Errorf("email reply: load upcoming visit: %w", err)
	}
	pendingVisit, err := a.appointmentViewer.GetPendingVisit(ctx, leadID, organizationID)
	if err != nil && !apperr.Is(err, apperr.KindNotFound) {
		return nil, nil, fmt.Errorf("email reply: load pending visit: %w", err)
	}
	return upcomingVisit, pendingVisit, nil
}

func (a *EmailReplyAgent) loadRequester(ctx context.Context, requesterUserID uuid.UUID) (*ports.ReplyUserProfile, error) {
	if a == nil || a.userReader == nil || requesterUserID == uuid.Nil {
		return nil, nil
	}
	requester, err := a.userReader.GetUserProfile(ctx, requesterUserID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load requester: %w", err)
	}
	return requester, nil
}

func (a *EmailReplyAgent) loadLatestAIAnalysis(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.AIAnalysis, error) {
	analysis, err := a.repo.GetLatestAIAnalysis(ctx, serviceID, organizationID)
	if err == nil {
		return &analysis, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("email reply: load ai analysis: %w", err)
}

func (a *EmailReplyAgent) loadLatestVisitReport(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.AppointmentVisitReport, error) {
	visitReport, err := a.repo.GetLatestAppointmentVisitReportByService(ctx, serviceID, organizationID)
	if err == nil {
		return visitReport, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("email reply: load visit report: %w", err)
}

func (a *EmailReplyAgent) loadLatestPhotoAnalysis(ctx context.Context, serviceID, organizationID uuid.UUID) (*repository.PhotoAnalysis, error) {
	photoAnalysis, err := a.repo.GetLatestPhotoAnalysis(ctx, serviceID, organizationID)
	if err == nil {
		return &photoAnalysis, nil
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrPhotoAnalysisNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("email reply: load photo analysis: %w", err)
}

func buildEmailReplyPrompt(input ports.EmailReplyInput, replyContext emailReplyContext, toneOfVoice string) string {
	return fmt.Sprintf(`Lead context
- Lead ID: %s
- Naam: %s
- Adres: %s
- Wooncontext: %s
- E-mailadres klant: %s

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

Current customer email
- Naam: %s
- Onderwerp: %s
- Bericht: %s

Task
- Draft one Dutch email reply for the company to send to this customer.
- Keep the reply customer-ready and suitable to paste into an email composer.
- Use a natural greeting when a name is available.
- Do not add a subject line.
- If a non-generic reply scenario is selected, treat that scenario as the primary drafting goal.
- Use the timing context to distinguish clearly between past and future appointments, deadlines, and updates.
- Use the intent hints and communication style hints as steering context when they fit the email.
- Use the real examples as style guidance when they fit, but never copy them literally.
- Use older notes or analysis only when they clearly improve the answer.
- If lead or service context is missing, rely only on the current email and ask a minimal clarifying question instead of assuming CRM history.
- If something important is missing, ask at most 2 concrete questions.
- Do not mention internal systems, AI analysis, or that this is a draft.
- Do not invent prices, promises, schedules, or technical facts that are not in the context.
- Output only the email body text.
`,
		formatEmailReplyLeadID(replyContext.lead),
		formatEmailReplyLeadName(replyContext.lead),
		formatEmailReplyLeadAddress(replyContext.lead),
		formatEmailReplyHousingContext(replyContext.lead),
		sanitizePromptField(input.CustomerEmail, 160),
		formatEmailReplyServiceID(replyContext.service),
		formatEmailReplyServiceType(replyContext.service),
		formatEmailReplyServiceStatus(replyContext.service),
		formatEmailReplyServicePipelineStage(replyContext.service),
		formatEmailReplyConsumerNote(replyContext.service),
		formatEmailReplyCustomerPreferences(replyContext.service),
		sanitizePromptField(toneOfVoice, 200),
		formatCurrentDateTimeBlock(),
		formatReplyScenarioBlock(input.Scenario, input.ScenarioNotes),
		formatEmailIntentBlock(input),
		formatEmailStyleSummary(input),
		formatRequesterBlock(replyContext.requester),
		formatQuoteOverviewBlock(replyContext.acceptedQuote),
		formatAgendaOverviewBlock(replyContext.upcomingVisit, replyContext.pendingVisit),
		formatUnknownsBlock(replyContext.service, replyContext.analysis, replyContext.acceptedQuote, replyContext.upcomingVisit, replyContext.pendingVisit, replyContext.lead != nil),
		formatTimelineBlock(replyContext.recentTimeline),
		formatAIAnalysisBlock(replyContext.analysis),
		formatVisitReportBlock(replyContext.visitReport),
		formatPhotoAnalysisBlock(replyContext.photoAnalysis),
		formatLeadNotesBlock(replyContext.notes),
		formatEmailFeedbackMemory(input.Feedback),
		formatEmailExamples(input.Examples),
		sanitizePromptField(input.CustomerName, 120),
		sanitizePromptField(input.Subject, 240),
		sanitizePromptField(input.MessageBody, maxEmailBodyChars),
	)
}

func emailReplySystemPrompt(toneOfVoice string) string {
	if strings.TrimSpace(toneOfVoice) == "" {
		toneOfVoice = ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	}
	return strings.TrimSpace(fmt.Sprintf(`## Tenant Tone Addendum

- Match this tenant tone of voice: %s.`, sanitizePromptField(toneOfVoice, 200)))
}

func formatEmailFeedbackMemory(items []ports.EmailReplyFeedback) string {
	if len(items) == 0 {
		return valueNotProvided
	}

	limit := len(items)
	if limit > maxEmailExampleItems {
		limit = maxEmailExampleItems
	}
	lines := make([]string, 0, limit*2)
	for _, item := range items[:limit] {
		aiReply := sanitizePromptField(item.AIReply, maxEmailFeedbackBodyChars)
		humanReply := sanitizePromptField(item.HumanReply, maxEmailFeedbackBodyChars)
		if aiReply == valueNotProvided || humanReply == valueNotProvided {
			continue
		}
		lines = append(lines,
			fmt.Sprintf("- [%s] AI-draft: %s", item.CreatedAt.Format(dateTimeLayout), aiReply),
			fmt.Sprintf("  Menselijke correctie: %s", humanReply),
		)
	}
	if len(lines) == 0 {
		return valueNotProvided
	}
	return strings.Join(lines, "\n")
}

func formatEmailReplyLeadID(lead *repository.Lead) string {
	if lead == nil || lead.ID == uuid.Nil {
		return valueNotProvided
	}
	return lead.ID.String()
}

func formatEmailReplyLeadName(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadName(*lead)
}

func formatEmailReplyLeadAddress(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadAddress(*lead)
}

func formatEmailReplyHousingContext(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return buildLeadHousingContext(*lead)
}

func formatEmailReplyServiceID(service *repository.LeadService) string {
	if service == nil || service.ID == uuid.Nil {
		return valueNotProvided
	}
	return service.ID.String()
}

func formatEmailReplyServiceType(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.ServiceType, 120)
}

func formatEmailReplyServiceStatus(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.Status, 80)
}

func formatEmailReplyServicePipelineStage(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.PipelineStage, 80)
}

func formatEmailReplyConsumerNote(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return optionalPromptString(service.ConsumerNote, 800)
}

func formatEmailReplyCustomerPreferences(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return formatJSONBlock(service.CustomerPreferences)
}

func formatEmailExamples(examples []ports.EmailReplyExample) string {
	if len(examples) == 0 {
		return valueNotProvided
	}
	limit := len(examples)
	if limit > maxEmailExampleItems {
		limit = maxEmailExampleItems
	}
	lines := make([]string, 0, limit*2)
	for _, example := range examples[:limit] {
		lines = append(lines,
			fmt.Sprintf("- [%s] Klant: %s", example.CreatedAt.Format(dateTimeLayout), sanitizePromptField(example.CustomerMessage, maxEmailFeedbackBodyChars)),
			fmt.Sprintf("  Bedrijf: %s", sanitizePromptField(example.Reply, maxEmailFeedbackBodyChars)),
		)
	}
	return strings.Join(lines, "\n")
}
