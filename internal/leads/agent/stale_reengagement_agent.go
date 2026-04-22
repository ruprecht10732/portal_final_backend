package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/orchestration"
	"portal_final_backend/platform/ai/openaicompat"
)

const (
	staleReEngagementAppName   = "stale-reengagement"
	maxReEngageTimelineItems   = 5
	maxReEngageNoteItems       = 5
	maxReEngageNoteChars       = 300
	maxReEngageSummaryChars    = 500
	maxReEngageContactMsgChars = 600
)

// StaleReEngagementAgent generates AI-powered re-engagement suggestions for
// stale lead services. It analyses the lead's history (timeline, notes,
// analysis, pipeline stage) and produces a recommended action with a draft
// message that the operator can send directly.
type StaleReEngagementAgent struct {
	repo                         repository.LeadsRepository
	modelConfig                  openaicompat.Config
	appName                      string
	organizationAISettingsReader ports.OrganizationAISettingsReader
	sessionService               session.Service
}

// StaleReEngagementResult is the structured output from the LLM.
type StaleReEngagementResult struct {
	RecommendedAction       string `json:"recommended_action"`
	SuggestedContactMessage string `json:"suggested_contact_message"`
	PreferredContactChannel string `json:"preferred_contact_channel"`
	Summary                 string `json:"summary"`
}

// NewStaleReEngagementAgent creates a new agent instance.
func NewStaleReEngagementAgent(modelCfg openaicompat.Config, repo repository.LeadsRepository, sessionService session.Service) *StaleReEngagementAgent {
	return &StaleReEngagementAgent{
		repo:           repo,
		modelConfig:    modelCfg,
		appName:        staleReEngagementAppName,
		sessionService: sessionService,
	}
}

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader.
func (a *StaleReEngagementAgent) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	a.organizationAISettingsReader = reader
}

// GenerateSuggestion produces a re-engagement suggestion for a single stale
// lead service.
func (a *StaleReEngagementAgent) GenerateSuggestion(
	ctx context.Context,
	orgID, leadID, serviceID uuid.UUID,
	staleReason string,
) (StaleReEngagementResult, error) {
	lead, service, notes, timeline, analysis, err := a.loadContext(ctx, orgID, leadID, serviceID)
	if err != nil {
		return StaleReEngagementResult{}, fmt.Errorf("stale reengagement: load context: %w", err)
	}

	settings := a.loadSettings(ctx, orgID)

	r, err := a.newRunner(settings.WhatsAppToneOfVoice)
	if err != nil {
		return StaleReEngagementResult{}, fmt.Errorf("stale reengagement: create runner: %w", err)
	}

	promptText := buildStaleReEngagementPrompt(
		lead, service, notes, timeline, analysis,
		staleReason, settings.WhatsAppToneOfVoice,
	)

	sessionID := "stale-reengage-" + serviceID.String()
	userID := "stale-reengage-" + orgID.String()

	outputText, err := runPromptTextSession(ctx, promptRunRequest{
		SessionService:       a.sessionService,
		Runner:               r,
		AppName:              a.appName,
		UserID:               userID,
		SessionID:            sessionID,
		CreateSessionMessage: "stale reengagement: create session",
		RunFailureMessage:    "stale reengagement: run failed",
		TraceLabel:           staleReEngagementAppName,
	}, promptText)
	if err != nil {
		return StaleReEngagementResult{}, err
	}

	return parseStaleReEngagementResponse(outputText)
}

func (a *StaleReEngagementAgent) newRunner(toneOfVoice string) (*runner.Runner, error) {
	kimi := openaicompat.NewModel(a.modelConfig)
	instruction, err := orchestration.BuildAgentInstruction(staleReEngagementAppName, staleReEngagementSystemPrompt(toneOfVoice))
	if err != nil {
		return nil, fmt.Errorf("stale reengagement: load workspace: %w", err)
	}

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        "StaleReEngagementAgent",
		Model:       kimi,
		Description: "Generates a re-engagement recommendation and draft message for a stale lead service.",
		Instruction: instruction,
	})
	if err != nil {
		return nil, err
	}

	r, err := runner.New(runner.Config{
		AppName:        a.appName,
		SessionService: a.sessionService,
		Agent:          adkAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("stale reengagement: create runner: %w", err)
	}
	return r, nil
}

func (a *StaleReEngagementAgent) loadSettings(ctx context.Context, organizationID uuid.UUID) ports.OrganizationAISettings {
	settings := ports.DefaultOrganizationAISettings()
	if a == nil || a.organizationAISettingsReader == nil {
		return settings
	}
	loaded, err := a.organizationAISettingsReader(ctx, organizationID)
	if err != nil {
		log.Printf("stale reengagement: failed to load org AI settings for %s: %v", organizationID, err)
		return settings
	}
	if strings.TrimSpace(loaded.WhatsAppToneOfVoice) == "" {
		loaded.WhatsAppToneOfVoice = settings.WhatsAppToneOfVoice
	}
	return loaded
}

func (a *StaleReEngagementAgent) loadContext(
	ctx context.Context,
	orgID, leadID, serviceID uuid.UUID,
) (
	*repository.Lead,
	*repository.LeadService,
	[]repository.LeadNote,
	[]repository.TimelineEvent,
	*repository.AIAnalysis,
	error,
) {
	lead, err := a.repo.GetByID(ctx, leadID, orgID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load lead: %w", err)
	}

	service, err := a.repo.GetLeadServiceByID(ctx, serviceID, orgID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load service: %w", err)
	}

	notes, err := a.repo.ListNotesByService(ctx, leadID, serviceID, orgID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load notes: %w", err)
	}

	timeline, err := loadRecentTimelineEvents(ctx, a.repo, leadID, &serviceID, orgID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load timeline: %w", err)
	}

	analysis, _ := a.repo.GetLatestAIAnalysis(ctx, serviceID, orgID)
	var analysisPtr *repository.AIAnalysis
	if analysis.ID != uuid.Nil {
		analysisPtr = &analysis
	}

	return &lead, &service, notes, timeline, analysisPtr, nil
}

// ---------- Prompt helpers ----------

func staleReEngagementSystemPrompt(toneOfVoice string) string {
	if strings.TrimSpace(toneOfVoice) == "" {
		toneOfVoice = ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	}
	return strings.TrimSpace(fmt.Sprintf(`## Tenant Tone Addendum

- Match this tenant tone of voice: %s.`, sanitizePromptField(toneOfVoice, 200)))
}

func buildStaleReEngagementPrompt(
	lead *repository.Lead,
	service *repository.LeadService,
	notes []repository.LeadNote,
	timeline []repository.TimelineEvent,
	analysis *repository.AIAnalysis,
	staleReason string,
	toneOfVoice string,
) string {
	return fmt.Sprintf(`Lead context
- Naam: %s
- Telefoon: %s
- E-mail: %s
- Adres: %s

Service context
- Service type: %s
- Status: %s
- Pipeline stage: %s
- Consumer note: %s

Stale reason
- Reden: %s
- Toelichting: %s

Current date and time
%s

Tone of voice
%s

Latest AI analysis
%s

Recent timeline
%s

Lead notes
%s

Task
Analyseer deze stagnerende lead en genereer een re-engagement aanbeveling.
Antwoord in EXACT het volgende JSON-formaat (geen extra tekst, alleen valid JSON):
{
  "recommended_action": "<korte actie in het Nederlands, bijv. 'Bel de klant', 'Stuur een WhatsApp', 'Verstuur de offerte', 'Plan een afspraak'>",
  "suggested_contact_message": "<een kant-en-klare berichten-draft in het Nederlands die de operator direct kan versturen>",
  "preferred_contact_channel": "<'whatsapp' of 'email' of 'phone'>",
  "summary": "<korte interne samenvatting in het Nederlands van max 2 zinnen waarom deze actie wordt aanbevolen>"
}

Richtlijnen:
- De aanbevolen actie moet concreet en uitvoerbaar zijn.
- Het concept-bericht moet warm, persoonlijk en professioneel zijn.
- Baseer het bericht op de beschikbare context (naam, servicetype, reden stagnatie).
- Verwijs NIET naar interne systemen, AI-analyse of dat dit een concept is.
- Verzin GEEN prijzen, afspraken of technische feiten die niet in de context staan.
- Kies het contactkanaal op basis van beschikbare contactgegevens en eerdere communicatie.
- Als er een telefoonnummer is, geef de voorkeur aan WhatsApp. Als er alleen een e-mail is, kies e-mail.
- Houd het bericht kort (max 3-4 zinnen voor WhatsApp, iets langer voor e-mail).
`,
		formatReEngageLeadName(lead),
		formatReEngagePhone(lead),
		formatReEngageEmail(lead),
		formatReEngageAddress(lead),
		formatReEngageServiceType(service),
		formatReEngageServiceStatus(service),
		formatReEngagePipelineStage(service),
		formatReEngageConsumerNote(service),
		sanitizePromptField(staleReason, 100),
		staleReasonExplanation(staleReason),
		formatCurrentDateTimeBlock(),
		sanitizePromptField(toneOfVoice, 200),
		formatReEngageAnalysis(analysis),
		formatReEngageTimeline(timeline),
		formatReEngageNotes(notes),
	)
}

func formatReEngageLeadName(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return joinPromptFields(
		sanitizePromptField(lead.ConsumerFirstName, 100),
		sanitizePromptField(lead.ConsumerLastName, 100),
	)
}

func formatReEngagePhone(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return sanitizePromptField(lead.ConsumerPhone, 30)
}

func formatReEngageEmail(lead *repository.Lead) string {
	if lead == nil || lead.ConsumerEmail == nil {
		return valueNotProvided
	}
	return sanitizePromptField(*lead.ConsumerEmail, 200)
}

func formatReEngageAddress(lead *repository.Lead) string {
	if lead == nil {
		return valueNotProvided
	}
	return joinPromptFields(
		sanitizePromptField(lead.AddressStreet, 100),
		sanitizePromptField(lead.AddressHouseNumber, 20),
		sanitizePromptField(lead.AddressZipCode, 20),
		sanitizePromptField(lead.AddressCity, 80),
	)
}

func formatReEngageServiceType(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.ServiceType, 100)
}

func formatReEngageServiceStatus(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.Status, 50)
}

func formatReEngagePipelineStage(service *repository.LeadService) string {
	if service == nil {
		return valueNotProvided
	}
	return sanitizePromptField(service.PipelineStage, 50)
}

func formatReEngageConsumerNote(service *repository.LeadService) string {
	if service == nil || service.ConsumerNote == nil {
		return valueNotProvided
	}
	return sanitizePromptField(*service.ConsumerNote, maxConsumerNote)
}

func staleReasonExplanation(reason string) string {
	switch reason {
	case "no_activity":
		return "Geen activiteit in de afgelopen 7 dagen"
	case "stuck_nurturing":
		return "Al meer dan 7 dagen in status 'Attempted Contact' zonder voortgang"
	case "no_quote_sent":
		return "In fase Estimation/Proposal maar geen offerte verstuurd na 14 dagen"
	case "stale_draft":
		return "Er staat een offerte-concept open dat al 30 dagen niet is verstuurd"
	case "needs_rescheduling":
		return "Afspraak moet worden verzet en staat al 2+ dagen open"
	default:
		return "Onbekende reden"
	}
}

func formatReEngageAnalysis(analysis *repository.AIAnalysis) string {
	if analysis == nil {
		return valueNotProvided
	}
	var sb strings.Builder
	sb.WriteString("- Urgentie: ")
	sb.WriteString(sanitizePromptField(analysis.UrgencyLevel, 20))
	sb.WriteString("\n- Aanbevolen actie: ")
	sb.WriteString(sanitizePromptField(analysis.RecommendedAction, 200))
	sb.WriteString("\n- Samenvatting: ")
	sb.WriteString(sanitizePromptField(analysis.Summary, maxReEngageSummaryChars))
	if analysis.SuggestedContactMessage != "" {
		sb.WriteString("\n- Eerder voorgesteld bericht: ")
		sb.WriteString(sanitizePromptField(analysis.SuggestedContactMessage, maxReEngageContactMsgChars))
	}
	return sb.String()
}

func formatReEngageTimeline(events []repository.TimelineEvent) string {
	if len(events) == 0 {
		return valueNotProvided
	}
	var sb strings.Builder
	limit := maxReEngageTimelineItems
	if limit > len(events) {
		limit = len(events)
	}
	for i := 0; i < limit; i++ {
		e := events[i]
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "- [%s] %s: %s",
			e.CreatedAt.Format(dateTimeLayout),
			sanitizePromptField(e.EventType, 50),
			sanitizePromptField(e.Title, 200),
		)
	}
	return sb.String()
}

func formatReEngageNotes(notes []repository.LeadNote) string {
	if len(notes) == 0 {
		return valueNotProvided
	}
	var sb strings.Builder
	limit := maxReEngageNoteItems
	if limit > len(notes) {
		limit = len(notes)
	}
	for i := 0; i < limit; i++ {
		n := notes[i]
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		content := sanitizePromptField(n.Body, maxReEngageNoteChars)
		fmt.Fprintf(&sb, "- [%s] %s", n.CreatedAt.Format(dateTimeLayout), content)
	}
	return sb.String()
}

// ---------- Response parsing ----------

func parseStaleReEngagementResponse(raw string) (StaleReEngagementResult, error) {
	text := strings.TrimSpace(raw)

	// Strip markdown code fences if present
	if strings.HasPrefix(text, "```") {
		if idx := strings.Index(text[3:], "\n"); idx >= 0 {
			text = text[3+idx+1:]
		}
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var result StaleReEngagementResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return StaleReEngagementResult{}, fmt.Errorf("stale reengagement: parse response: %w (raw: %s)", err, truncate(text, 200))
	}

	if result.RecommendedAction == "" {
		return StaleReEngagementResult{}, fmt.Errorf("stale reengagement: empty recommended_action")
	}
	if result.SuggestedContactMessage == "" {
		return StaleReEngagementResult{}, fmt.Errorf("stale reengagement: empty suggested_contact_message")
	}

	// Normalize channel
	switch strings.ToLower(result.PreferredContactChannel) {
	case "whatsapp", "email", "phone":
		result.PreferredContactChannel = strings.ToLower(result.PreferredContactChannel)
	default:
		result.PreferredContactChannel = "whatsapp"
	}

	if result.Summary == "" {
		result.Summary = result.RecommendedAction
	}

	return result, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
