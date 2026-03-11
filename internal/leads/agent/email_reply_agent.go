package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"

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
}

type emailReplyContext struct {
	lead          *repository.Lead
	service       *repository.LeadService
	notes         []repository.LeadNote
	analysis      *repository.AIAnalysis
	visitReport   *repository.AppointmentVisitReport
	photoAnalysis *repository.PhotoAnalysis
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

func (a *EmailReplyAgent) SuggestEmailReply(ctx context.Context, input ports.EmailReplyInput) (string, error) {
	replyContext, err := a.loadReplyContext(ctx, input)
	if err != nil {
		return "", err
	}
	settings := a.loadOrganizationAISettings(ctx, input.OrganizationID)
	r, sessionService, err := a.newRunner(settings.WhatsAppToneOfVoice)
	if err != nil {
		return "", err
	}

	promptText := buildEmailReplyPrompt(input, replyContext, settings.WhatsAppToneOfVoice)
	sessionID := uuid.NewString()
	userID := "email-reply-" + input.OrganizationID.String() + ":" + sanitizeUserInput(strings.ToLower(strings.TrimSpace(input.CustomerEmail)), 120)

	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   a.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("email reply: create session: %w", err)
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
	if err := consumeRunEvents(r.Run(ctx, userID, sessionID, userMessage, runConfig), "email reply: run failed", func(event *session.Event) {
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

func (a *EmailReplyAgent) newRunner(toneOfVoice string) (*runner.Runner, session.Service, error) {
	kimi := moonshot.NewModel(a.modelConfig)
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "EmailReplyAgent",
		Model:       kimi,
		Description: "Suggests a single email reply draft grounded in lead and service context.",
		Instruction: emailReplySystemPrompt(toneOfVoice),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create email reply agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        a.appName,
		Agent:          adkAgent,
		SessionService: sessionService,
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
	lead, err := a.loadReplyLead(ctx, input)
	if err != nil {
		return emailReplyContext{}, err
	}

	service, err := a.loadReplyService(ctx, input, lead)
	if err != nil {
		return emailReplyContext{}, err
	}

	if lead == nil && service != nil && service.LeadID != uuid.Nil {
		loadedLead, loadErr := a.repo.GetByID(ctx, service.LeadID, input.OrganizationID)
		if loadErr == nil {
			lead = &loadedLead
		} else if !errors.Is(loadErr, repository.ErrNotFound) {
			return emailReplyContext{}, fmt.Errorf("email reply: load lead from service: %w", loadErr)
		}
	}

	contextData := emailReplyContext{
		lead:    lead,
		service: service,
		notes:   nil,
	}
	if service == nil {
		return contextData, nil
	}

	leadID := service.LeadID
	if lead != nil && lead.ID != uuid.Nil {
		leadID = lead.ID
	}

	notes, err := a.repo.ListNotesByService(ctx, leadID, service.ID, input.OrganizationID)
	if err != nil {
		return emailReplyContext{}, fmt.Errorf("email reply: load notes: %w", err)
	}
	contextData.notes = notes

	analysis, err := a.loadLatestAIAnalysis(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return emailReplyContext{}, err
	}
	visitReport, err := a.loadLatestVisitReport(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return emailReplyContext{}, err
	}
	photoAnalysis, err := a.loadLatestPhotoAnalysis(ctx, service.ID, input.OrganizationID)
	if err != nil {
		return emailReplyContext{}, err
	}

	contextData.analysis = analysis
	contextData.visitReport = visitReport
	contextData.photoAnalysis = photoAnalysis
	return contextData, nil
}

func (a *EmailReplyAgent) loadReplyLead(ctx context.Context, input ports.EmailReplyInput) (*repository.Lead, error) {
	if input.LeadID != nil && *input.LeadID != uuid.Nil {
		lead, err := a.repo.GetByID(ctx, *input.LeadID, input.OrganizationID)
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

	summary, _, err := a.repo.GetByPhoneOrEmail(ctx, "", input.CustomerEmail, input.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("email reply: load lead by email: %w", err)
	}
	if summary == nil || summary.ID == uuid.Nil {
		return nil, nil
	}

	lead, err := a.repo.GetByID(ctx, summary.ID, input.OrganizationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load lead: %w", err)
	}
	return &lead, nil
}

func (a *EmailReplyAgent) loadReplyService(ctx context.Context, input ports.EmailReplyInput, lead *repository.Lead) (*repository.LeadService, error) {
	if input.LeadServiceID != nil && *input.LeadServiceID != uuid.Nil {
		service, err := a.repo.GetLeadServiceByID(ctx, *input.LeadServiceID, input.OrganizationID)
		if err == nil {
			return &service, nil
		}
		if !errors.Is(err, repository.ErrServiceNotFound) && !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("email reply: load lead service: %w", err)
		}
	}

	if lead == nil || lead.ID == uuid.Nil {
		return nil, nil
	}

	service, err := a.repo.GetCurrentLeadService(ctx, lead.ID, input.OrganizationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrServiceNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("email reply: load current lead service: %w", err)
	}
	return &service, nil
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
- Use the real examples as style guidance when they fit, but never copy them literally.
- Use older notes or analysis only when they clearly improve the answer.
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
	return strings.TrimSpace(fmt.Sprintf(`You write customer-ready email replies for a Dutch home-services company.

Rules:
- Return exactly one draft reply in Dutch.
- Keep it concise, professional, and ready to send.
- Match this tenant tone of voice: %s.
- Write in plain email body text only.
- Include a natural salutation when the customer name is available.
- Do not include a subject line.
- Ground the reply in the provided lead, service, and email context.
- If the customer asks a direct question, answer it directly when the context supports it.
- If details are still needed, ask at most two clear questions and explain briefly why.
- Never expose internal reasoning, raw analysis data, or uncertainty labels.
- Never fabricate pricing, availability, measurements, or policy details.
- Output only the reply text, with no title or surrounding quotes.`, sanitizePromptField(toneOfVoice, 200)))
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
