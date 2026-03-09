package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/ai/moonshot"
)

const (
	whatsAppReplyAppName        = "whatsapp-reply-generator"
	maxWhatsAppTranscriptItems  = 12
	maxWhatsAppMessageBodyChars = 750
)

type WhatsAppReplyAgent struct {
	repo           repository.LeadsRepository
	runner         *runner.Runner
	sessionService session.Service
	appName        string
}

type whatsAppReplyContext struct {
	lead          repository.Lead
	service       repository.LeadService
	notes         []repository.LeadNote
	analysis      *repository.AIAnalysis
	visitReport   *repository.AppointmentVisitReport
	photoAnalysis *repository.PhotoAnalysis
}

func NewWhatsAppReplyAgent(apiKey string, modelName string, repo repository.LeadsRepository) (*WhatsAppReplyAgent, error) {
	kimi := moonshot.NewModel(newMoonshotModelConfig(apiKey, modelName))

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "WhatsAppReplyAgent",
		Model:       kimi,
		Description: "Suggests a single WhatsApp reply draft grounded in lead and conversation context.",
		Instruction: whatsappReplySystemPrompt(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create whatsapp reply agent: %w", err)
	}

	sessionService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        whatsAppReplyAppName,
		Agent:          adkAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create whatsapp reply runner: %w", err)
	}

	return &WhatsAppReplyAgent{
		repo:           repo,
		runner:         r,
		sessionService: sessionService,
		appName:        whatsAppReplyAppName,
	}, nil
}

func (a *WhatsAppReplyAgent) SuggestWhatsAppReply(ctx context.Context, input ports.WhatsAppReplyInput) (string, error) {
	replyContext, err := a.loadReplyContext(ctx, input)
	if err != nil {
		return "", err
	}

	promptText := buildWhatsAppReplyPrompt(input, replyContext.lead, replyContext.service, replyContext.notes, replyContext.analysis, replyContext.visitReport, replyContext.photoAnalysis)
	sessionID := input.ConversationID.String()
	userID := "whatsapp-reply-" + input.LeadID.String()

	_, err = a.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   a.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("whatsapp reply: create session: %w", err)
	}
	defer func() {
		_ = a.sessionService.Delete(ctx, &session.DeleteRequest{
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
	if err := consumeRunEvents(a.runner.Run(ctx, userID, sessionID, userMessage, runConfig), "whatsapp reply: run failed", func(event *session.Event) {
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

	return whatsAppReplyContext{
		lead:          lead,
		service:       service,
		notes:         notes,
		analysis:      analysis,
		visitReport:   visitReport,
		photoAnalysis: photoAnalysis,
	}, nil
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
	lead repository.Lead,
	service repository.LeadService,
	notes []repository.LeadNote,
	analysis *repository.AIAnalysis,
	visitReport *repository.AppointmentVisitReport,
	photoAnalysis *repository.PhotoAnalysis,
) string {
	return fmt.Sprintf(`Lead context
- Lead ID: %s
- Naam: %s
- Telefoon: %s
- E-mail: %s
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

Latest AI analysis
%s

Latest visit report
%s

Latest photo analysis
%s

Lead notes
%s

Recent WhatsApp transcript
%s

Task
- Draft one WhatsApp reply in Dutch for the company to send to this customer.
- Use the lead and service context to make the reply specific and useful.
- Keep the message ready to paste into WhatsApp.
- If something important is missing, ask at most 2 concrete questions.
- Do not mention internal systems, AI analysis, or that this is a draft.
- Do not invent prices, promises, or technical facts that are not in the context.
- Output only the reply text.
`,
		lead.ID,
		buildLeadName(lead),
		sanitizePromptField(lead.ConsumerPhone, 64),
		optionalPromptString(lead.ConsumerEmail, 120),
		buildLeadAddress(lead),
		buildLeadHousingContext(lead),
		sanitizePromptField(input.DisplayName, 120),
		service.ID,
		sanitizePromptField(service.ServiceType, 120),
		sanitizePromptField(service.Status, 80),
		sanitizePromptField(service.PipelineStage, 80),
		optionalPromptString(service.ConsumerNote, 800),
		formatJSONBlock(service.CustomerPreferences),
		formatAIAnalysisBlock(analysis),
		formatVisitReportBlock(visitReport),
		formatPhotoAnalysisBlock(photoAnalysis),
		formatLeadNotesBlock(notes),
		formatWhatsAppTranscript(input.Messages),
	)
}

func whatsappReplySystemPrompt() string {
	return strings.TrimSpace(`You write customer-ready WhatsApp replies for a Dutch home-services company.

Rules:
- Return exactly one draft reply in Dutch.
- Keep it concise, warm, and practical.
- Prefer short paragraphs suitable for WhatsApp.
- Ground the reply in the provided lead, service, and conversation context.
- If the latest customer message asks a direct question, answer it directly when the context supports it.
- If details are still needed, ask at most two clear questions and explain briefly why.
- Never expose internal reasoning, raw analysis data, or uncertainty labels.
- Never fabricate pricing, availability, measurements, or policy details.
- Output only the message text, with no title or surrounding quotes.`)
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
		fmt.Sprintf("- Samenvatting: %s", sanitizePromptField(analysis.Summary, 800)),
		fmt.Sprintf("- Aanbevolen actie: %s", sanitizePromptField(analysis.RecommendedAction, 80)),
		fmt.Sprintf("- Voorkeurskanaal: %s", sanitizePromptField(analysis.PreferredContactChannel, 32)),
	}
	if len(analysis.MissingInformation) > 0 {
		parts = append(parts, fmt.Sprintf("- Ontbrekende info: %s", sanitizeUserInput(strings.Join(analysis.MissingInformation, "; "), 600)))
	}
	if len(analysis.ResolvedInformation) > 0 {
		parts = append(parts, fmt.Sprintf("- Reeds bevestigd: %s", sanitizeUserInput(strings.Join(analysis.ResolvedInformation, "; "), 600)))
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
	if len(notes) > 4 {
		start = len(notes) - 4
	}
	lines := make([]string, 0, len(notes)-start)
	for _, note := range notes[start:] {
		body := sanitizePromptField(note.Body, maxNoteLength)
		lines = append(lines, fmt.Sprintf("- %s (%s)", body, note.CreatedAt.Format(dateTimeLayout)))
	}
	return strings.Join(lines, "\n")
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
