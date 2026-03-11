package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/apperr"
)

const (
	tenantReplyTimezone   = "Europe/Amsterdam"
	maxTimelineItems      = 5
	maxQuoteScopeItems    = 5
	maxUnknownItems       = 6
	maxStyleSignalSamples = 6
	freshnessLineFormat   = "- Actualiteit: %s"
	noteGroupImportant    = "Belangrijk"
	noteGroupPreferences  = "Voorkeuren"
	noteGroupAccess       = "Toegang/planning"
	noteGroupOther        = "Overig"
)

type inferredIntent struct {
	label   string
	urgency string
	reason  string
}

func loadRecentTimelineEvents(ctx context.Context, repo repository.LeadsRepository, leadID uuid.UUID, serviceID *uuid.UUID, organizationID uuid.UUID) ([]repository.TimelineEvent, error) {
	if leadID == uuid.Nil {
		return nil, nil
	}

	var (
		items []repository.TimelineEvent
		err   error
	)
	if serviceID != nil && *serviceID != uuid.Nil {
		items, err = repo.ListTimelineEventsByService(ctx, leadID, *serviceID, organizationID)
	} else {
		items, err = repo.ListTimelineEvents(ctx, leadID, organizationID)
	}
	if err != nil {
		return nil, err
	}

	filtered := make([]repository.TimelineEvent, 0, maxTimelineItems)
	for _, item := range items {
		if strings.EqualFold(item.Visibility, repository.TimelineVisibilityDebug) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= maxTimelineItems {
			break
		}
	}
	return filtered, nil
}

func attachAppointmentAssigneeNames(ctx context.Context, userReader ports.ReplyUserReader, appointments ...*ports.PublicAppointmentSummary) error {
	if userReader == nil {
		return nil
	}
	for _, appointment := range appointments {
		if shouldSkipAppointmentAssignee(appointment) {
			continue
		}
		name, err := resolveAppointmentAssigneeName(ctx, userReader, *appointment.AssignedUserID)
		if err != nil {
			return err
		}
		if name != "" {
			appointment.AssignedUserName = &name
		}
	}
	return nil
}

func shouldSkipAppointmentAssignee(appointment *ports.PublicAppointmentSummary) bool {
	return appointment == nil || appointment.AssignedUserID == nil || *appointment.AssignedUserID == uuid.Nil
}

func resolveAppointmentAssigneeName(ctx context.Context, userReader ports.ReplyUserReader, userID uuid.UUID) (string, error) {
	profile, err := userReader.GetUserProfile(ctx, userID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return "", nil
		}
		return "", err
	}
	if profile == nil {
		return "", nil
	}
	name := joinPromptFields(optionalPromptString(profile.FirstName, 80), optionalPromptString(profile.LastName, 80))
	if name == valueNotProvided {
		name = sanitizePromptField(profile.Email, 160)
	}
	if name == valueNotProvided {
		return "", nil
	}
	return name, nil
}

func replyLocalNow() time.Time {
	loc, err := time.LoadLocation(tenantReplyTimezone)
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

func formatCurrentDateTimeBlock() string {
	now := replyLocalNow()
	return strings.Join([]string{
		"- Nu: " + now.Format(time.RFC3339),
		"- Lokale planningstijdzone: " + tenantReplyTimezone,
		"- Gebruik dit om te bepalen of afspraken, deadlines en gebeurtenissen in het verleden of de toekomst liggen.",
	}, "\n")
}

func formatFreshness(ts time.Time) string {
	if ts.IsZero() {
		return valueNotProvided
	}
	now := replyLocalNow()
	diff := now.Sub(ts)
	abs := diff
	if abs < 0 {
		abs = -abs
	}

	var rel string
	switch {
	case abs < time.Minute:
		rel = "zojuist"
	case abs < time.Hour:
		mins := int(abs.Minutes())
		if diff >= 0 {
			rel = fmt.Sprintf("%d min geleden", mins)
		} else {
			rel = fmt.Sprintf("over %d min", mins)
		}
	case abs < 24*time.Hour:
		hours := int(abs.Hours())
		if diff >= 0 {
			rel = fmt.Sprintf("%d uur geleden", hours)
		} else {
			rel = fmt.Sprintf("over %d uur", hours)
		}
	default:
		days := int(abs.Hours() / 24)
		if diff >= 0 {
			rel = fmt.Sprintf("%d dagen geleden", days)
		} else {
			rel = fmt.Sprintf("over %d dagen", days)
		}
	}
	return fmt.Sprintf("%s (%s)", ts.In(now.Location()).Format(dateTimeLayout), rel)
}

func formatTimelineBlock(items []repository.TimelineEvent) string {
	if len(items) == 0 {
		return valueNotProvided
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		title := sanitizePromptField(item.Title, 120)
		summary := optionalPromptString(item.Summary, 220)
		if summary == valueNotProvided {
			summary = sanitizePromptField(item.EventType, 80)
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", formatFreshness(item.CreatedAt), title, summary))
	}
	return strings.Join(lines, "\n")
}

func formatUnknownsBlock(service *repository.LeadService, analysis *repository.AIAnalysis, quote *ports.PublicQuoteSummary, upcomingVisit, pendingVisit *ports.PublicAppointmentSummary, hasLead bool) string {
	items := make([]string, 0, maxUnknownItems)
	items = append(items, baseUnknowns(service, hasLead)...)
	items = append(items, analysisUnknowns(analysis, remainingUnknownSlots(items))...)
	items = append(items, planningUnknowns(quote, upcomingVisit, pendingVisit, remainingUnknownSlots(items))...)
	if len(items) == 0 {
		return valueNotProvided
	}
	if len(items) > maxUnknownItems {
		items = items[:maxUnknownItems]
	}
	return strings.Join(items, "\n")
}

func baseUnknowns(service *repository.LeadService, hasLead bool) []string {
	items := []string{}
	if !hasLead {
		items = append(items, "- Geen gekoppelde leadcontext beschikbaar")
	}
	if service == nil {
		items = append(items, "- Geen actieve dienstcontext beschikbaar")
	}
	return items
}

func analysisUnknowns(analysis *repository.AIAnalysis, remaining int) []string {
	if analysis == nil || remaining <= 0 {
		return nil
	}
	items := make([]string, 0, remaining)
	for _, missing := range limitPromptList(analysis.MissingInformation, remaining) {
		trimmed := sanitizePromptField(missing, 180)
		if trimmed != valueNotProvided {
			items = append(items, "- "+trimmed)
		}
	}
	return items
}

func planningUnknowns(quote *ports.PublicQuoteSummary, upcomingVisit, pendingVisit *ports.PublicAppointmentSummary, remaining int) []string {
	if remaining <= 0 {
		return nil
	}
	items := []string{}
	if quote == nil {
		items = append(items, "- Geen geaccepteerde offerte gevonden")
	}
	if upcomingVisit == nil && pendingVisit == nil && len(items) < remaining {
		items = append(items, "- Geen toekomstige afspraak of open afspraakverzoek bekend")
	}
	if len(items) > remaining {
		return items[:remaining]
	}
	return items
}

func remainingUnknownSlots(items []string) int {
	if len(items) >= maxUnknownItems {
		return 0
	}
	return maxUnknownItems - len(items)
}

func formatWhatsAppIntentBlock(input ports.WhatsAppReplyInput) string {
	message := latestInboundWhatsAppText(input.Messages)
	if message == "" {
		return valueNotProvided
	}
	intent := inferIntent(message, "")
	return formatIntent(intent)
}

func formatEmailIntentBlock(input ports.EmailReplyInput) string {
	intent := inferIntent(input.MessageBody, input.Subject)
	return formatIntent(intent)
}

func formatIntent(intent inferredIntent) string {
	lines := []string{}
	if intent.label != "" {
		lines = append(lines, "- Primair doel: "+intent.label)
	}
	if intent.urgency != "" {
		lines = append(lines, "- Urgentie: "+intent.urgency)
	}
	if intent.reason != "" {
		lines = append(lines, "- Signalen: "+sanitizePromptField(intent.reason, 220))
	}
	if len(lines) == 0 {
		return valueNotProvided
	}
	return strings.Join(lines, "\n")
}

func inferIntent(body, subject string) inferredIntent {
	text := strings.ToLower(strings.TrimSpace(subject + "\n" + body))
	if text == "" {
		return inferredIntent{}
	}

	intent := inferredIntent{label: "algemene opvolging", urgency: "normaal"}
	var reasons []string

	containsAny := func(needles ...string) bool {
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return true
			}
		}
		return false
	}

	switch {
	case containsAny("klacht", "ontevreden", "teleurgesteld", "niet ok", "werkt niet", "jammer"):
		intent.label = "klacht of probleem"
		reasons = append(reasons, "ontevredenheids- of probleemsignalen")
	case containsAny("offerte", "prijs", "kosten", "bedrag", "akkoord", "geaccepteerd"):
		intent.label = "offerte of prijs"
		reasons = append(reasons, "offerte- of prijswoorden")
	case containsAny("afspraak", "planning", "wanneer", "langskomen", "beschikbaar", "morgen", "volgende week"):
		intent.label = "planning of afspraak"
		reasons = append(reasons, "planning- of afspraakwoorden")
	case containsAny("hoe", "wat", "welke", "kunnen jullie", "is het mogelijk", "?", "vraag"):
		intent.label = "informatievraag"
		reasons = append(reasons, "vraagformulering")
	case containsAny("prima", "doorgaan", "bevestig", "is goed", "helemaal goed"):
		intent.label = "bevestiging"
		reasons = append(reasons, "bevestigende taal")
	}

	if containsAny("spoed", "dringend", "vandaag", "zsm", "asap") {
		intent.urgency = "hoog"
		reasons = append(reasons, "spoedwoorden")
	}
	intent.reason = strings.Join(reasons, "; ")
	return intent
}

func formatWhatsAppStyleSummary(input ports.WhatsAppReplyInput) string {
	samples := make([]string, 0, maxStyleSignalSamples)
	for _, example := range input.Examples {
		samples = append(samples, example.Reply)
		if len(samples) >= maxStyleSignalSamples {
			break
		}
	}
	if len(samples) == 0 {
		for i := len(input.Messages) - 1; i >= 0 && len(samples) < maxStyleSignalSamples; i-- {
			if strings.EqualFold(strings.TrimSpace(input.Messages[i].Direction), "outbound") {
				samples = append(samples, input.Messages[i].Body)
			}
		}
	}
	return formatStyleSignals(samples, true)
}

func formatEmailStyleSummary(input ports.EmailReplyInput) string {
	samples := make([]string, 0, maxStyleSignalSamples)
	for _, example := range input.Examples {
		samples = append(samples, example.Reply)
		if len(samples) >= maxStyleSignalSamples {
			break
		}
	}
	return formatStyleSignals(samples, false)
}

func formatStyleSignals(samples []string, allowEmoji bool) string {
	if len(samples) == 0 {
		return valueNotProvided
	}
	stats := collectStyleStats(samples, allowEmoji)
	brevity := describeBrevity(stats.avgLen)
	formality := describeFormality(stats.formalSignals, stats.informalSignals)
	lines := []string{
		"- Gemiddelde stijl: " + brevity,
		"- Formaliteit: " + formality,
	}
	if stats.paragraphSignals > 0 {
		lines = append(lines, "- Structuur: gebruikt vaak korte alinea's")
	}
	if allowEmoji {
		if stats.emojiSignals > 0 {
			lines = append(lines, "- Emoji's: worden soms gebruikt")
		} else {
			lines = append(lines, "- Emoji's: liever spaarzaam")
		}
	}
	return strings.Join(lines, "\n")
}

type styleStats struct {
	avgLen           int
	formalSignals    int
	informalSignals  int
	emojiSignals     int
	paragraphSignals int
}

func collectStyleStats(samples []string, allowEmoji bool) styleStats {
	stats := styleStats{}
	count := 0
	for _, sample := range samples {
		text := strings.TrimSpace(sample)
		if text == "" {
			continue
		}
		count++
		stats.avgLen += len(text)
		lower := strings.ToLower(text)
		stats.formalSignals += boolToInt(hasFormalSignal(lower))
		stats.informalSignals += boolToInt(hasInformalSignal(lower))
		stats.paragraphSignals += boolToInt(strings.Contains(text, "\n\n"))
		if allowEmoji {
			stats.emojiSignals += boolToInt(containsEmojiLike(text))
		}
	}
	stats.avgLen = stats.avgLen / max(1, count)
	return stats
}

func hasFormalSignal(text string) bool {
	return strings.Contains(text, " geachte") || strings.Contains(text, " beste") || strings.Contains(text, "uw") || strings.Contains(text, "u ")
}

func hasInformalSignal(text string) bool {
	return strings.Contains(text, "hoi") || strings.Contains(text, "hey") || strings.Contains(text, "je") || strings.Contains(text, "jij")
}

func describeBrevity(avgLen int) string {
	switch {
	case avgLen < 100:
		return "kort en direct"
	case avgLen > 260:
		return "uitgebreid"
	default:
		return "middel"
	}
}

func describeFormality(formalSignals, informalSignals int) string {
	if formalSignals > informalSignals {
		return "formeel"
	}
	if informalSignals > formalSignals {
		return "informeel"
	}
	return "neutraal"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func containsEmojiLike(text string) bool {
	for _, marker := range []string{"🙂", "😊", "👍", "👋", "✅", "😉", "😄", "🙏", ":)", ";)"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func latestInboundWhatsAppText(messages []ports.WhatsAppReplyMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(messages[i].Direction), "outbound") {
			return strings.TrimSpace(messages[i].Body)
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return strings.TrimSpace(messages[len(messages)-1].Body)
}

func formatReplyScenarioBlock(scenario ports.ReplySuggestionScenario, notes string) string {
	normalized := ports.NormalizeReplySuggestionScenario(string(scenario))
	lines := []string{"- Geselecteerd scenario: " + replyScenarioLabel(normalized)}
	if noteSummary := sanitizePromptField(notes, 500); noteSummary != valueNotProvided {
		lines = append(lines, "- Extra operatorinstructie: "+noteSummary)
	}
	if guidance := replyScenarioGuidance(normalized); guidance != valueNotProvided {
		lines = append(lines, guidance)
	}
	return strings.Join(lines, "\n")
}

func replyScenarioLabel(scenario ports.ReplySuggestionScenario) string {
	switch scenario {
	case ports.ReplySuggestionScenarioFollowUp:
		return "follow-up zonder reactie"
	case ports.ReplySuggestionScenarioAppointmentReminder:
		return "afspraakherinnering"
	case ports.ReplySuggestionScenarioAppointmentConfirm:
		return "afspraakbevestiging"
	case ports.ReplySuggestionScenarioRescheduleRequest:
		return "afspraak verzetten"
	case ports.ReplySuggestionScenarioQuoteReminder:
		return "offerte opvolgen"
	case ports.ReplySuggestionScenarioQuoteExpiry:
		return "offerte verloopt bijna"
	case ports.ReplySuggestionScenarioMissingInformation:
		return "ontbrekende informatie opvragen"
	case ports.ReplySuggestionScenarioPhotosOrDocuments:
		return "foto's of documenten opvragen"
	case ports.ReplySuggestionScenarioPostVisitFollowUp:
		return "opvolging na bezoek"
	case ports.ReplySuggestionScenarioAcceptedQuoteNext:
		return "vervolgstappen na akkoord"
	case ports.ReplySuggestionScenarioDelayUpdate:
		return "vertraging of statusupdate"
	case ports.ReplySuggestionScenarioComplaintRecovery:
		return "klacht of herstelreactie"
	default:
		return "algemene reply"
	}
}

func replyScenarioGuidance(scenario ports.ReplySuggestionScenario) string {
	switch scenario {
	case ports.ReplySuggestionScenarioFollowUp:
		return "- Scenario-richting: wees kort, laagdrempelig en vraag om een kleine concrete reactie of volgende stap."
	case ports.ReplySuggestionScenarioAppointmentReminder:
		return "- Scenario-richting: herinner aan datum, tijd en praktische details zonder nieuwe onzekerheid toe te voegen."
	case ports.ReplySuggestionScenarioAppointmentConfirm:
		return "- Scenario-richting: bevestig de afspraak duidelijk en benoem alleen de relevante logistiek."
	case ports.ReplySuggestionScenarioRescheduleRequest:
		return "- Scenario-richting: leg kort uit dat verzetten nodig is en stuur op nieuwe opties of beschikbaarheid."
	case ports.ReplySuggestionScenarioQuoteReminder:
		return "- Scenario-richting: verwijs naar de offerte, houd de toon behulpzaam en vermijd druk zetten."
	case ports.ReplySuggestionScenarioQuoteExpiry:
		return "- Scenario-richting: benoem de geldigheid en nodig uit tot vragen of een besluit zonder hard sales-taal."
	case ports.ReplySuggestionScenarioMissingInformation:
		return "- Scenario-richting: vraag alleen de minimale set ontbrekende gegevens die nodig is om verder te kunnen."
	case ports.ReplySuggestionScenarioPhotosOrDocuments:
		return "- Scenario-richting: vraag expliciet welke foto's of documenten nodig zijn en waarom."
	case ports.ReplySuggestionScenarioPostVisitFollowUp:
		return "- Scenario-richting: sluit aan op het recente bezoek en stuur op de eerstvolgende logische stap."
	case ports.ReplySuggestionScenarioAcceptedQuoteNext:
		return "- Scenario-richting: bevestig dat er akkoord is en leg praktisch uit wat hierna gebeurt."
	case ports.ReplySuggestionScenarioDelayUpdate:
		return "- Scenario-richting: wees transparant over de vertraging, bied context en manage verwachtingen."
	case ports.ReplySuggestionScenarioComplaintRecovery:
		return "- Scenario-richting: erken het probleem eerst, vermijd defensieve taal en bied een oplossingsroute."
	default:
		return valueNotProvided
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
