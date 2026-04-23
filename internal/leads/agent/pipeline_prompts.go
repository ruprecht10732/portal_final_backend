package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
)

const noPreferencesProvided = "No preferences provided"

const (
	maxGatekeeperServiceNoteChars = 2000
	maxGatekeeperNotesChars       = 3000
	maxGatekeeperVisitReportChars = 2200
	maxGatekeeperPreferencesChars = 1200
	maxGatekeeperPhotoChars       = 2500
	maxGatekeeperLeadCtxChars     = 1200
	maxGatekeeperIntakeChars      = 3000

	maxEstimatorServiceNoteChars = 2000
	maxEstimatorNotesChars       = 3000
	maxEstimatorPreferencesChars = 1200
	maxEstimatorPhotoChars       = 3500
	maxEstimatorContextChars     = 6000

	maxQuoteServiceNoteChars = 2000
	maxQuoteNotesChars       = 2500
	maxQuotePreferencesChars = 1200
	maxQuoteUserPromptChars  = 1500
)

const extraNotesLinePrefix = "\n- Extra notes: "

var sharedExecutionContract = mustReadPromptFile("shared/prompts/execution-contract.md")

var sharedProductSelectionRules = mustReadPromptFile("shared/prompts/product-selection.md")

var sharedCommunicationContract = mustReadPromptFile("shared/prompts/communication-contract.md")

var sharedGlobalPreamble = mustReadPromptFile("shared/prompts/global-preamble.md")

var sharedMathExamples = mustReadPromptFile("shared/prompts/math-examples.md")

var sharedPhotoTrustRules = mustReadPromptFile("shared/prompts/photo-trust-rules.md")

var sharedIntakeCompletenessGate = mustReadPromptFile("shared/prompts/intake-completeness-gate.md")

var sharedWhatsAppFormatting = mustReadPromptFile("shared/prompts/whatsapp-formatting.md")

type gatekeeperPromptInput struct {
	lead               repository.Lead
	service            repository.LeadService
	notes              []repository.LeadNote
	visitReport        *repository.AppointmentVisitReport
	intakeContext      string
	estimationContext  string
	attachments        []repository.Attachment
	photoAnalysis      *repository.PhotoAnalysis
	priorAnalysis      *repository.AIAnalysis
	nurturingLoopCount int
	agentCycleCount    int
}

type quotePromptInput struct {
	lead              repository.Lead
	service           repository.LeadService
	notes             []repository.LeadNote
	photoAnalysis     *repository.PhotoAnalysis
	estimationContext string
	scopeArtifact     *ScopeArtifact
}

type gatekeeperPromptTemplateData struct {
	ExecutionContract         string
	CommunicationContract     string
	SharedPhotoTrustRules     string
	PreferredChannel          string
	RecoveryModeSection       string
	CycleAwarenessSection     string
	LeadID                    uuid.UUID
	ServiceID                 uuid.UUID
	ServiceType               string
	PipelineStage             string
	CreatedAt                 string
	ConsumerSummary           string
	LocationSummary           string
	ServiceNoteSummary        string
	NotesSection              string
	VisitReportSummary        string
	PreferencesSummary        string
	PhotoSummary              string
	PreviousEstimatorBlockers string
	KnownFacts                string
	AttachmentAwareness       string
	LeadContext               string
	IntakeContextSummary      string
	EstimationContextSummary  string
}

var gatekeeperPromptTemplate = mustLoadPromptTemplateWithPreamble("gatekeeper", "gatekeeper/prompts/base.md")

func buildGatekeeperPrompt(input gatekeeperPromptInput) string {
	notesSection := buildNotesSection(input.notes, maxGatekeeperNotesChars)
	visitReportSummary := truncatePromptSection(buildVisitReportSummary(input.visitReport), maxGatekeeperVisitReportChars)
	serviceNote := getValue(input.service.ConsumerNote)
	preferredChannel := resolvePreferredContactChannel(input.lead)
	preferencesSummary := buildPreferencesSummary(input.service.CustomerPreferences, maxGatekeeperPreferencesChars)
	leadContext := truncatePromptSection(buildLeadContextSection(input.lead, input.attachments), maxGatekeeperLeadCtxChars)
	attachmentAwareness := truncatePromptSection(buildAttachmentAwarenessSection(input.attachments), maxGatekeeperLeadCtxChars)
	photoSummary := truncatePromptSection(buildGatekeeperPhotoSummary(input.photoAnalysis, input.service.ServiceType), maxGatekeeperPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxGatekeeperServiceNoteChars)
	intakeContextSummary := truncatePromptSection(input.intakeContext, maxGatekeeperIntakeChars)
	estimationContextSummary := truncatePromptSection(input.estimationContext, maxGatekeeperIntakeChars)
	previousEstimatorBlockers := buildPreviousEstimatorBlockersSection(input.priorAnalysis)
	knownFacts := buildKnownFactsSection(input.priorAnalysis, input.visitReport)
	consumerSummary := buildPromptConsumerSection(input.lead)
	locationSummary := buildPromptLocationLine(input.lead)
	recoveryModeSection := ""
	if input.nurturingLoopCount > 1 {
		recoveryModeSection = fmt.Sprintf(`

=== RECOVERY MODE ===
[MANDATORY] The customer already tried to provide information, but it was still insufficient (Attempt %d).
[MANDATORY] Do NOT send a generic request.
[MANDATORY] Explicitly acknowledge the previous reply or photo before asking for anything else.
[MANDATORY] Explain exactly why the previous information was not enough, for example visibility, angle, shadow, missing scale, or missing measurement.
[MANDATORY] Offer an alternative path when helpful, such as a short call or a specialist visit if the customer cannot provide the requested detail.
`, input.nurturingLoopCount)
	}

	cycleAwarenessSection := ""
	if input.agentCycleCount > 0 {
		cycleAwarenessSection = fmt.Sprintf(`

=== CROSS-AGENT CYCLE WARNING ===
[MANDATORY] This service has bounced between Gatekeeper and Estimator %d time(s).
[MANDATORY] The Estimator has previously returned this service to Nurturing because it could not produce an estimate.
[MANDATORY] Do NOT advance to Estimation UNLESS the specific blockers listed in "Previous Estimator Blockers" are explicitly resolved with new information.
[MANDATORY] If blockers cannot be resolved with available information, set stage to Manual_Intervention with a clear explanation.
`, input.agentCycleCount)
	}

	return renderPromptTemplate(gatekeeperPromptTemplate, gatekeeperPromptTemplateData{
		ExecutionContract:         sharedExecutionContract,
		CommunicationContract:     sharedCommunicationContract,
		SharedPhotoTrustRules:     sharedPhotoTrustRules,
		PreferredChannel:          preferredChannel,
		RecoveryModeSection:       recoveryModeSection,
		CycleAwarenessSection:     cycleAwarenessSection,
		LeadID:                    input.lead.ID,
		ServiceID:                 input.service.ID,
		ServiceType:               input.service.ServiceType,
		PipelineStage:             input.service.PipelineStage,
		CreatedAt:                 input.lead.CreatedAt.Format(time.RFC3339),
		ConsumerSummary:           consumerSummary,
		LocationSummary:           locationSummary,
		ServiceNoteSummary:        serviceNoteSummary,
		NotesSection:              notesSection,
		VisitReportSummary:        visitReportSummary,
		PreferencesSummary:        preferencesSummary,
		PhotoSummary:              photoSummary,
		PreviousEstimatorBlockers: previousEstimatorBlockers,
		KnownFacts:                knownFacts,
		AttachmentAwareness:       attachmentAwareness,
		LeadContext:               leadContext,
		IntakeContextSummary:      intakeContextSummary,
		EstimationContextSummary:  estimationContextSummary,
	})
}

func buildPreviousEstimatorBlockersSection(priorAnalysis *repository.AIAnalysis) string {
	if priorAnalysis == nil {
		return "- Geen eerdere estimatorblokkades gevonden."
	}

	lines := make([]string, 0, 5)
	if action := strings.TrimSpace(priorAnalysis.RecommendedAction); action != "" {
		lines = append(lines, fmt.Sprintf("- Laatste aanbevolen actie: %s", action))
	}

	missingInformation := compactPromptList(priorAnalysis.MissingInformation)
	if len(missingInformation) > 0 {
		lines = append(lines, fmt.Sprintf("- Eerder ontbrekende intakegegevens: %s", strings.Join(missingInformation, ", ")))
	}

	riskFlags := compactPromptList(priorAnalysis.RiskFlags)
	if len(riskFlags) > 0 {
		lines = append(lines, fmt.Sprintf("- Risicosignalen: %s", strings.Join(riskFlags, ", ")))
	}

	if priorAnalysis.CompositeConfidence != nil {
		lines = append(lines, fmt.Sprintf("- Confidence vorige analyse: %.2f", *priorAnalysis.CompositeConfidence))
	}

	if summary := strings.TrimSpace(priorAnalysis.Summary); summary != "" {
		lines = append(lines, fmt.Sprintf("- Samenvatting vorige analyse: %s", sanitizeUserInput(summary, maxNoteLength)))
	}

	if len(lines) == 0 {
		return "- Geen eerdere estimatorblokkades gevonden."
	}

	return strings.Join(lines, "\n")
}

func buildKnownFactsSection(priorAnalysis *repository.AIAnalysis, visitReport *repository.AppointmentVisitReport) string {
	lines := make([]string, 0, 8)
	if priorAnalysis != nil {
		lines = append(lines, buildPriorAnalysisKnownFacts(priorAnalysis)...)
	}
	if visitReport != nil {
		if measurements := visitReportValue(visitReport.Measurements); measurements != valueNotProvided {
			lines = append(lines, fmt.Sprintf("- Ingemeten tijdens afspraak: %s", measurements))
		}
	}
	if len(lines) == 0 {
		return "- Geen duurzame bekende feiten opgeslagen."
	}
	return strings.Join(lines, "\n")
}

func buildPriorAnalysisKnownFacts(priorAnalysis *repository.AIAnalysis) []string {
	lines := make([]string, 0, len(priorAnalysis.ResolvedInformation)+len(priorAnalysis.ExtractedFacts)+1)
	resolvedInformation := compactPromptList(priorAnalysis.ResolvedInformation)
	if len(resolvedInformation) > 0 {
		lines = append(lines, fmt.Sprintf("- Eerder bevestigde intakegegevens: %s", strings.Join(resolvedInformation, ", ")))
	}
	return append(lines, buildExtractedFactLines(priorAnalysis.ExtractedFacts)...)
}

func buildExtractedFactLines(facts map[string]string) []string {
	if len(facts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(facts[key])
		if value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- Feit %s: %s", sanitizeUserInput(key, maxNoteLength), sanitizeUserInput(value, maxNoteLength)))
	}
	return lines
}

func compactPromptList(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		compacted = append(compacted, sanitizeUserInput(trimmed, maxNoteLength))
	}
	return compacted
}

func buildVisitReportSummary(report *repository.AppointmentVisitReport) string {
	if report == nil {
		return "No visit report available."
	}

	lines := []string{
		"- Measurements: " + visitReportValue(report.Measurements),
		"- Access difficulty: " + visitReportValue(report.AccessDifficulty),
		"- Notes: " + visitReportValue(report.Notes),
	}

	return wrapUserData(strings.Join(lines, "\n"))
}

func visitReportValue(value *string) string {
	if value == nil {
		return valueNotProvided
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return valueNotProvided
	}
	return sanitizeUserInput(trimmed, maxNoteLength)
}

func buildScopeAnalyzerPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis) string {
	notesSection := buildNotesSection(notes, maxEstimatorNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxEstimatorPreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxEstimatorServiceNoteChars)

	return renderPromptTemplate(scopeAnalyzerPromptTemplate, struct {
		ExecutionContract           string
		SharedPhotoTrustRules       string
		SharedIntakeCompletenessGate string
		LeadID                      uuid.UUID
		ServiceID                   uuid.UUID
		ServiceType                 string
		PipelineStage               string
		ServiceNoteSummary          string
		NotesSection                string
		PreferencesSummary          string
		PhotoSummary                string
	}{
		ExecutionContract:            sharedExecutionContract,
		SharedPhotoTrustRules:        sharedPhotoTrustRules,
		SharedIntakeCompletenessGate: sharedIntakeCompletenessGate,
		LeadID:                       lead.ID,
		ServiceID:                    service.ID,
		ServiceType:                  service.ServiceType,
		PipelineStage:                service.PipelineStage,
		ServiceNoteSummary:           serviceNoteSummary,
		NotesSection:                 notesSection,
		PreferencesSummary:           preferencesSummary,
		PhotoSummary:                 photoSummary,
	})
}

func buildQuoteBuilderPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis, estimationContext string, scopeArtifact *ScopeArtifact) string {
	notesSection := buildNotesSection(notes, maxEstimatorNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxEstimatorPreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxEstimatorServiceNoteChars)
	estimationContextSummary := truncatePromptSection(estimationContext, maxEstimatorContextChars)
	scopeSummary := truncatePromptSection(formatScopeArtifact(scopeArtifact), maxEstimatorContextChars)
	consumerSummary := buildPromptConsumerSection(lead)
	locationSummary := buildPromptLocationLine(lead)

	return renderPromptTemplate(quoteBuilderPromptTemplate, struct {
		ExecutionContract            string
		ScopeSummary                 string
		SharedProductSelectionRules  string
		SharedMathExamples           string
		SharedPhotoTrustRules        string
		SharedIntakeCompletenessGate string
		LeadID                       uuid.UUID
		ServiceID                    uuid.UUID
		ServiceType                  string
		PipelineStage                string
		CreatedAt                    string
		ConsumerSummary              string
		LocationSummary              string
		ServiceNoteSummary           string
		NotesSection                 string
		PreferencesSummary           string
		PhotoSummary                 string
		EstimationContextSummary     string
	}{
		ExecutionContract:            sharedExecutionContract,
		ScopeSummary:                 scopeSummary,
		SharedProductSelectionRules:  sharedProductSelectionRules,
		SharedMathExamples:           sharedMathExamples,
		SharedPhotoTrustRules:        sharedPhotoTrustRules,
		SharedIntakeCompletenessGate: sharedIntakeCompletenessGate,
		LeadID:                       lead.ID,
		ServiceID:                    service.ID,
		ServiceType:                  service.ServiceType,
		PipelineStage:                service.PipelineStage,
		CreatedAt:                    lead.CreatedAt.Format(time.RFC3339),
		ConsumerSummary:              consumerSummary,
		LocationSummary:              locationSummary,
		ServiceNoteSummary:           serviceNoteSummary,
		NotesSection:                 notesSection,
		PreferencesSummary:           preferencesSummary,
		PhotoSummary:                 photoSummary,
		EstimationContextSummary:     estimationContextSummary,
	})
}

func buildInvestigativePrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis, missingItems []string, estimationContext string) string {
	notesSection := buildNotesSection(notes, maxEstimatorNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferredChannel := resolvePreferredContactChannel(lead)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxEstimatorPreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxEstimatorServiceNoteChars)
	estimationContextSummary := truncatePromptSection(estimationContext, maxEstimatorContextChars)
	houseContextSummary := truncatePromptSection(buildHouseContextSection(lead), maxGatekeeperLeadCtxChars)

	missing := "- Geen expliciete lijst ontvangen"
	if len(missingItems) > 0 {
		rows := make([]string, 0, len(missingItems))
		for _, item := range missingItems {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			rows = append(rows, "- "+trimmed)
		}
		if len(rows) > 0 {
			missing = strings.Join(rows, "\n")
		}
	}

	return renderPromptTemplate(investigativePromptTemplate, struct {
		ExecutionContract        string
		CommunicationContract    string
		Missing                  string
		PreferredChannel         string
		LeadID                   uuid.UUID
		ServiceID                uuid.UUID
		ServiceType              string
		ServiceNoteSummary       string
		NotesSection             string
		PreferencesSummary       string
		PhotoSummary             string
		HouseContextSummary      string
		EstimationContextSummary string
	}{
		ExecutionContract:        sharedExecutionContract,
		CommunicationContract:    sharedCommunicationContract,
		Missing:                  missing,
		PreferredChannel:         preferredChannel,
		LeadID:                   lead.ID,
		ServiceID:                service.ID,
		ServiceType:              service.ServiceType,
		ServiceNoteSummary:       serviceNoteSummary,
		NotesSection:             notesSection,
		PreferencesSummary:       preferencesSummary,
		PhotoSummary:             photoSummary,
		HouseContextSummary:      houseContextSummary,
		EstimationContextSummary: estimationContextSummary,
	})
}

func buildHouseContextSection(lead repository.Lead) string {
	return wrapUserData(strings.Join([]string{
		"Energy: " + buildEnergySummary(lead),
		"Enrichment: " + buildEnrichmentSummary(lead),
	}, "\n"))
}

func resolvePreferredContactChannel(lead repository.Lead) string {
	if strings.TrimSpace(lead.ConsumerPhone) != "" {
		return "WhatsApp"
	}
	return "Email"
}

func formatScopeArtifact(scopeArtifact *ScopeArtifact) string {
	if scopeArtifact == nil {
		return "No scope artifact committed."
	}
	b, err := json.MarshalIndent(scopeArtifact, "", "  ")
	if err != nil {
		return "Scope artifact available but could not be rendered."
	}
	return string(b)
}

func buildDispatcherPrompt(lead repository.Lead, service repository.LeadService, radiusKm int, excludeIDs []uuid.UUID) string {
	exclusionTxt := ""
	if len(excludeIDs) > 0 {
		exclusionTxt = fmt.Sprintf("\nCONTEXT: The following Partner IDs have already been contacted or rejected: %v. You MUST include these in the 'excludePartnerIds' field when calling FindMatchingPartners.", excludeIDs)
	}
	referenceData := wrapReferenceBlock(fmt.Sprintf(`Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s
- Pipeline Stage: %s
- Zip Code: %s%s`,
		lead.ID,
		service.ID,
		service.ServiceType,
		service.PipelineStage,
		lead.AddressZipCode,
		exclusionTxt,
	))

	return renderPromptTemplate(dispatcherPromptTemplate, struct {
		ExecutionContract string
		ReferenceData     string
		ServiceType       string
		ZipCode           string
		RadiusKm          int
	}{
		ExecutionContract: sharedExecutionContract,
		ReferenceData:     referenceData,
		ServiceType:       service.ServiceType,
		ZipCode:           lead.AddressZipCode,
		RadiusKm:          radiusKm,
	})
}

func buildNotesSection(notes []repository.LeadNote, maxChars int) string {
	if len(notes) == 0 {
		return "No notes"
	}

	sorted := sortNotesForPrompt(notes)
	contentBudget := resolveNotesContentBudget(maxChars)
	content := renderNotesWithinBudget(sorted, contentBudget)
	if strings.TrimSpace(content) == "" {
		return "No notes"
	}
	return wrapUserData(content)
}

type scoredNote struct {
	n repository.LeadNote
	p int
}

func scoreNoteForPrompt(n repository.LeadNote) int {
	nt := strings.ToLower(strings.TrimSpace(n.Type))
	body := strings.ToLower(n.Body)

	// Lowest priority: system/log style notes.
	if nt == "system" || strings.Contains(nt, "system") || strings.Contains(nt, "log") {
		return 100
	}

	// Highest priority: explicit contact and constraint notes.
	if strings.Contains(nt, "call") || strings.Contains(nt, "phone") || strings.Contains(nt, "contact") || strings.Contains(nt, "email") || strings.Contains(nt, "sms") || strings.Contains(nt, "whatsapp") {
		return 0
	}
	if strings.Contains(body, "bel") || strings.Contains(body, "call") || strings.Contains(body, "contact") || strings.Contains(body, "na ") || strings.Contains(body, "after") || strings.Contains(body, "alleen") || strings.Contains(body, "only") || strings.Contains(body, "allerg") {
		return 10
	}

	return 50
}

func sortNotesForPrompt(notes []repository.LeadNote) []repository.LeadNote {
	// Truncation blindness guard:
	// Sort newest-first so prompt budget pressure drops stale notes before recent ones.
	// Keep note priority only as a tie-breaker for identical timestamps.
	candidates := make([]scoredNote, 0, len(notes))
	for _, n := range notes {
		candidates = append(candidates, scoredNote{n: n, p: scoreNoteForPrompt(n)})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].n.CreatedAt.Equal(candidates[j].n.CreatedAt) {
			return candidates[i].n.CreatedAt.After(candidates[j].n.CreatedAt)
		}
		if candidates[i].p != candidates[j].p {
			return candidates[i].p < candidates[j].p
		}
		return false
	})

	sorted := make([]repository.LeadNote, 0, len(candidates))
	for _, c := range candidates {
		sorted = append(sorted, c.n)
	}
	return sorted
}

func resolveNotesContentBudget(maxChars int) int {
	if maxChars <= 0 {
		maxChars = maxEstimatorNotesChars
	}
	// Headroom because wrapUserData adds explicit instruction-boundary markers.
	contentBudget := maxChars - 192
	if contentBudget < 200 {
		contentBudget = maxChars
	}
	return contentBudget
}

func renderNotesWithinBudget(notes []repository.LeadNote, contentBudget int) string {
	var sb strings.Builder
	for _, n := range notes {
		body := sanitizeUserInput(n.Body, maxNoteLength)
		prefix := fmt.Sprintf("- [%s] %s: ", n.Type, n.CreatedAt.Format(time.RFC3339))
		line := prefix + body + "\n"

		if len([]rune(sb.String()+line)) <= contentBudget {
			sb.WriteString(line)
			continue
		}

		remaining := contentBudget - len([]rune(sb.String()+prefix+"\n"))
		if remaining <= 0 {
			break
		}
		truncated := strings.TrimSpace(truncateRunes(body, remaining))
		if truncated == "" {
			break
		}
		sb.WriteString(prefix + truncated + "... [afgekapt]\n")
		break
	}
	return sb.String()
}

func buildLeadContextSection(lead repository.Lead, attachments []repository.Attachment) string {
	energySummary := buildEnergySummary(lead)
	enrichmentSummary := buildEnrichmentSummary(lead)
	attachmentsSummary := buildAttachmentsSummary(attachments)

	return wrapUserData(strings.Join([]string{
		"Energy: " + energySummary,
		"Enrichment: " + enrichmentSummary,
		"Attachments: " + attachmentsSummary,
	}, "\n"))
}

func buildEnergySummary(lead repository.Lead) string {
	if lead.EnergyClass == nil && lead.EnergyIndex == nil && lead.EnergyBouwjaar == nil && lead.EnergyGebouwtype == nil {
		return "No energy label data"
	}

	parts := make([]string, 0, 4)
	if lead.EnergyClass != nil {
		parts = append(parts, "class "+*lead.EnergyClass)
	}
	if lead.EnergyIndex != nil {
		parts = append(parts, fmt.Sprintf("index %.2f", *lead.EnergyIndex))
	}
	if lead.EnergyBouwjaar != nil {
		parts = append(parts, fmt.Sprintf("build year %d", *lead.EnergyBouwjaar))
	}
	if lead.EnergyGebouwtype != nil {
		parts = append(parts, "type "+*lead.EnergyGebouwtype)
	}

	if len(parts) == 0 {
		return "No energy label data"
	}
	return strings.Join(parts, ", ")
}

func buildEnrichmentSummary(lead repository.Lead) string {
	parts := make([]string, 0, 4)
	if lead.LeadEnrichmentSource != nil {
		parts = append(parts, "source "+*lead.LeadEnrichmentSource)
	}
	if lead.LeadEnrichmentPostcode6 != nil {
		parts = append(parts, "postcode6 "+*lead.LeadEnrichmentPostcode6)
	}
	if lead.LeadEnrichmentBuurtcode != nil {
		parts = append(parts, "buurtcode "+*lead.LeadEnrichmentBuurtcode)
	}
	if lead.LeadEnrichmentConfidence != nil {
		parts = append(parts, fmt.Sprintf("confidence %.2f", *lead.LeadEnrichmentConfidence))
	}
	if len(parts) == 0 {
		return "No enrichment data"
	}
	return strings.Join(parts, ", ")
}

func buildAttachmentsSummary(attachments []repository.Attachment) string {
	if len(attachments) == 0 {
		return "No attachments"
	}

	names := make([]string, 0, 5)
	for i, att := range attachments {
		if i >= 5 {
			break
		}
		name := sanitizeUserInput(att.FileName, 80)
		kind, _, _ := classifyAttachment(att)
		names = append(names, fmt.Sprintf("%s [%s]", name, kind))
	}
	return fmt.Sprintf("%d file(s): %s", len(attachments), strings.Join(names, ", "))
}

func buildAttachmentAwarenessSection(attachments []repository.Attachment) string {
	if len(attachments) == 0 {
		return "- No attachments"
	}
	lines := make([]string, 0, len(attachments)+3)
	hasNonImageDocument := false
	requiresDocumentReview := false
	for i, att := range attachments {
		kind, isNonImageDocument, requiresReview := classifyAttachment(att)
		if isNonImageDocument {
			hasNonImageDocument = true
		}
		if requiresReview {
			requiresDocumentReview = true
		}
		if i < 5 {
			lines = append(lines, fmt.Sprintf("- %s [%s]", sanitizeUserInput(att.FileName, 80), kind))
		}
	}
	lines = append(lines, fmt.Sprintf("- Non-image documents detected: %t", hasNonImageDocument))
	lines = append(lines, fmt.Sprintf("- Human document review recommended: %t", requiresDocumentReview))
	if requiresDocumentReview {
		lines = append(lines, "- Reason: attachment set may already contain measurements, plans, or quote details that the AI cannot reliably read.")
	}
	return wrapUserData(strings.Join(lines, "\n"))
}

func classifyAttachment(att repository.Attachment) (kind string, isNonImageDocument bool, requiresDocumentReview bool) {
	contentType := strings.ToLower(strings.TrimSpace(getValue(att.ContentType)))
	ext := strings.ToLower(filepath.Ext(att.FileName))
	name := strings.ToLower(strings.TrimSpace(att.FileName))
	if strings.HasPrefix(contentType, "image/") || isImageExtension(ext) {
		return "image", false, false
	}
	if isDocumentAttachment(contentType, ext) {
		requiresReview := strings.Contains(name, "plattegrond") || strings.Contains(name, "floorplan") || strings.Contains(name, "blueprint") || strings.Contains(name, "tekening") || strings.Contains(name, "offerte") || strings.Contains(name, "quote") || contentType == "application/pdf" || ext == ".pdf"
		label := "document"
		if ext != "" {
			label = "document/" + strings.TrimPrefix(ext, ".")
		}
		return label, true, requiresReview
	}
	if contentType != "" {
		return contentType, true, true
	}
	return "file", ext != "", ext != ""
}

func isImageExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

func isDocumentAttachment(contentType string, ext string) bool {
	if strings.HasPrefix(contentType, "application/") || strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch ext {
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".rtf", ".odt", ".ods", ".csv":
		return true
	default:
		return false
	}
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func buildQuoteGeneratePrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, userPrompt string, estimationContext string) string {
	notesSection := buildNotesSection(notes, maxQuoteNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxQuotePreferencesChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxQuoteServiceNoteChars)
	userPromptSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(userPrompt, 2000)), maxQuoteUserPromptChars)
	estimationContextSummary := truncatePromptSection(estimationContext, maxEstimatorContextChars)
	consumerSummary := buildPromptConsumerSection(lead)
	locationSummary := buildPromptLocationLine(lead)
	referenceData := wrapReferenceBlock(fmt.Sprintf(`Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s

Consumer:
%s

Address:
%s

Service Note (raw):
%s

Notes:
%s

Preferences (from customer portal):
%s

Estimation Guidelines:
%s

User Prompt:
%s`,
		lead.ID,
		service.ID,
		service.ServiceType,
		consumerSummary,
		locationSummary,
		serviceNoteSummary,
		notesSection,
		preferencesSummary,
		estimationContextSummary,
		userPromptSummary,
	))

	return renderPromptTemplate(quoteGeneratePromptTemplate, struct {
		ExecutionContract           string
		SharedProductSelectionRules string
		SharedMathExamples          string
		ReferenceData               string
	}{
		ExecutionContract:           sharedExecutionContract,
		SharedProductSelectionRules: sharedProductSelectionRules,
		SharedMathExamples:          sharedMathExamples,
		ReferenceData:               referenceData,
	})
}

func buildQuoteCriticPrompt(input quotePromptInput, draftInput DraftQuoteInput, draftResult *ports.DraftQuoteResult) string {
	notesSection := buildNotesSection(input.notes, maxQuoteNotesChars)
	serviceNote := getValue(input.service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(input.service.CustomerPreferences, maxQuotePreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(input.photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxQuoteServiceNoteChars)
	estimationContextSummary := truncatePromptSection(input.estimationContext, maxEstimatorContextChars)
	scopeSummary := truncatePromptSection(formatScopeArtifact(input.scopeArtifact), maxEstimatorContextChars)
	consumerSummary := buildPromptConsumerSection(input.lead)
	locationSummary := buildPromptLocationLine(input.lead)
	draftJSON := formatDraftQuoteForCritic(draftInput)

	return renderPromptTemplate(quoteCriticPromptTemplate, struct {
		ExecutionContract        string
		LeadID                   uuid.UUID
		ServiceID                uuid.UUID
		QuoteID                  uuid.UUID
		QuoteNumber              string
		ServiceType              string
		ConsumerSummary          string
		LocationSummary          string
		ServiceNoteSummary       string
		NotesSection             string
		PreferencesSummary       string
		PhotoSummary             string
		ScopeSummary             string
		EstimationContextSummary string
		DraftJSON                string
	}{
		ExecutionContract:        sharedExecutionContract,
		LeadID:                   input.lead.ID,
		ServiceID:                input.service.ID,
		QuoteID:                  draftResult.QuoteID,
		QuoteNumber:              draftResult.QuoteNumber,
		ServiceType:              input.service.ServiceType,
		ConsumerSummary:          consumerSummary,
		LocationSummary:          locationSummary,
		ServiceNoteSummary:       serviceNoteSummary,
		NotesSection:             notesSection,
		PreferencesSummary:       preferencesSummary,
		PhotoSummary:             photoSummary,
		ScopeSummary:             scopeSummary,
		EstimationContextSummary: estimationContextSummary,
		DraftJSON:                draftJSON,
	})
}

func buildQuoteRepairPrompt(input quotePromptInput, draftInput DraftQuoteInput, critique SubmitQuoteCritiqueInput, attempt int) string {
	notesSection := buildNotesSection(input.notes, maxQuoteNotesChars)
	serviceNote := getValue(input.service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(input.service.CustomerPreferences, maxQuotePreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(input.photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxQuoteServiceNoteChars)
	estimationContextSummary := truncatePromptSection(input.estimationContext, maxEstimatorContextChars)
	scopeSummary := truncatePromptSection(formatScopeArtifact(input.scopeArtifact), maxEstimatorContextChars)
	consumerSummary := buildPromptConsumerSection(input.lead)
	locationSummary := buildPromptLocationLine(input.lead)
	draftJSON := formatDraftQuoteForCritic(draftInput)
	critiqueJSON := formatQuoteCritiqueForRepair(critique)

	return renderPromptTemplate(quoteRepairPromptTemplate, struct {
		ExecutionContract        string
		LeadID                   uuid.UUID
		ServiceID                uuid.UUID
		ServiceType              string
		Attempt                  int
		ConsumerSummary          string
		LocationSummary          string
		ServiceNoteSummary       string
		NotesSection             string
		PreferencesSummary       string
		PhotoSummary             string
		ScopeSummary             string
		EstimationContextSummary string
		DraftJSON                string
		CritiqueJSON             string
	}{
		ExecutionContract:        sharedExecutionContract,
		LeadID:                   input.lead.ID,
		ServiceID:                input.service.ID,
		ServiceType:              input.service.ServiceType,
		Attempt:                  attempt,
		ConsumerSummary:          consumerSummary,
		LocationSummary:          locationSummary,
		ServiceNoteSummary:       serviceNoteSummary,
		NotesSection:             notesSection,
		PreferencesSummary:       preferencesSummary,
		PhotoSummary:             photoSummary,
		ScopeSummary:             scopeSummary,
		EstimationContextSummary: estimationContextSummary,
		DraftJSON:                draftJSON,
		CritiqueJSON:             critiqueJSON,
	})
}

func formatDraftQuoteForCritic(input DraftQuoteInput) string {
	payload := struct {
		Notes string           `json:"notes,omitempty"`
		Items []DraftQuoteItem `json:"items"`
	}{
		Notes: input.Notes,
		Items: input.Items,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "Kon conceptofferte niet serialiseren voor review."
	}
	return string(b)
}

func formatQuoteCritiqueForRepair(input SubmitQuoteCritiqueInput) string {
	b, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "Kon critic findings niet serialiseren voor reparatie."
	}
	return string(b)
}

func truncatePromptSection(section string, maxChars int) string {
	if maxChars <= 0 {
		return section
	}
	runes := []rune(section)
	if len(runes) <= maxChars {
		return section
	}
	suffix := "\n...[truncated for token budget]"
	suffixRunes := []rune(suffix)
	keep := maxChars - len(suffixRunes)
	if keep <= 0 {
		return string(runes[:maxChars])
	}
	return string(runes[:keep]) + suffix
}

func buildPreferencesSummary(raw json.RawMessage, maxChars int) string {
	if len(raw) == 0 {
		return noPreferencesProvided
	}

	var prefs struct {
		Budget       string `json:"budget"`
		Timeframe    string `json:"timeframe"`
		Availability string `json:"availability"`
		ExtraNotes   string `json:"extraNotes"`
	}
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return noPreferencesProvided
	}

	budget := strings.TrimSpace(prefs.Budget)
	timeframe := strings.TrimSpace(prefs.Timeframe)
	availability := strings.TrimSpace(prefs.Availability)
	extraNotes := strings.TrimSpace(prefs.ExtraNotes)

	if budget == "" && timeframe == "" && availability == "" && extraNotes == "" {
		return noPreferencesProvided
	}

	// Truncation blindness guard: keep budget/timeframe/availability visible and
	// truncate extra notes first if we exceed the prompt budget.
	baseLines := []string{
		"- Budget: " + preferenceValue(budget),
		"- Timeframe: " + preferenceValue(timeframe),
		"- Availability: " + preferenceValue(availability),
	}
	content := strings.Join(baseLines, "\n")
	if extraNotes != "" {
		content = content + extraNotesLinePrefix + preferenceValue(extraNotes)
	}

	wrapped := wrapUserData(content)
	if maxChars > 0 && len([]rune(wrapped)) > maxChars {
		if extraNotes != "" {
			prefixWrapped := wrapUserData(strings.Join(baseLines, "\n") + extraNotesLinePrefix)
			available := maxChars - len([]rune(prefixWrapped))
			if available > 0 {
				trimmedExtra := truncateRunes(preferenceValue(extraNotes), available)
				content = strings.Join(baseLines, "\n") + extraNotesLinePrefix + trimmedExtra + "... [afgekapt]"
				wrapped = wrapUserData(content)
			}
		}
		if len([]rune(wrapped)) > maxChars {
			wrapped = truncatePromptSection(wrapped, maxChars)
		}
	}

	return wrapped
}

func preferenceValue(value string) string {
	if value == "" {
		return valueNotProvided
	}
	return sanitizeUserInput(value, maxNoteLength)
}

func buildPhotoSummaryContent(photoAnalysis *repository.PhotoAnalysis) string {
	var sb strings.Builder
	if photoAnalysis.Summary != "" {
		sb.WriteString("Summary: " + photoAnalysis.Summary + "\n")
	}
	if photoAnalysis.ScopeAssessment != "" {
		sb.WriteString("Scope: " + photoAnalysis.ScopeAssessment + "\n")
	}
	if photoAnalysis.CostIndicators != "" {
		sb.WriteString("Cost: " + photoAnalysis.CostIndicators + "\n")
	}
	if len(photoAnalysis.Observations) > 0 {
		sb.WriteString("Observations: " + strings.Join(photoAnalysis.Observations, "; ") + "\n")
	}
	if len(photoAnalysis.SafetyConcerns) > 0 {
		sb.WriteString("Safety: " + strings.Join(photoAnalysis.SafetyConcerns, "; ") + "\n")
	}
	if len(photoAnalysis.AdditionalInfo) > 0 {
		sb.WriteString("Additional: " + strings.Join(photoAnalysis.AdditionalInfo, "; ") + "\n")
	}
	if len(photoAnalysis.Measurements) > 0 || len(photoAnalysis.NeedsOnsiteMeasurement) > 0 {
		sb.WriteString("Measurement guardrail: Treat photo-derived dimensions as advisory only unless they are explicitly visible, labeled, or OCR-backed. On-site measurement requests override uncertain dimensions.\n")
	}

	// New v2 fields
	if len(photoAnalysis.Measurements) > 0 {
		sb.WriteString("Measurements:\n")
		for _, m := range photoAnalysis.Measurements {
			_, _ = fmt.Fprintf(&sb, "  - %s: %.2f %s (%s, confidence: %s)\n", m.Description, m.Value, m.Unit, m.Type, m.Confidence)
		}
	}
	if len(photoAnalysis.NeedsOnsiteMeasurement) > 0 {
		sb.WriteString("Needs on-site measurement: " + strings.Join(photoAnalysis.NeedsOnsiteMeasurement, "; ") + "\n")
	}
	if len(photoAnalysis.Discrepancies) > 0 {
		sb.WriteString("⚠ Discrepancies (consumer claims vs photos): " + strings.Join(photoAnalysis.Discrepancies, "; ") + "\n")
	}
	if len(photoAnalysis.ExtractedText) > 0 {
		sb.WriteString("Extracted text (OCR): " + strings.Join(photoAnalysis.ExtractedText, "; ") + "\n")
	}
	if len(photoAnalysis.SuggestedSearchTerms) > 0 {
		sb.WriteString("Suggested product search terms: " + strings.Join(photoAnalysis.SuggestedSearchTerms, ", ") + "\n")
	}

	return sb.String()
}

func buildPhotoSummary(photoAnalysis *repository.PhotoAnalysis) string {
	if photoAnalysis == nil {
		return "No photo analysis available."
	}

	return wrapUserData(buildPhotoSummaryContent(photoAnalysis))
}

func buildGatekeeperPhotoSummary(photoAnalysis *repository.PhotoAnalysis, serviceType string) string {
	if photoAnalysis == nil {
		return "No photo analysis available."
	}
	if isPhotoAnalysisLikelyIrrelevant(photoAnalysis) {
		details := strings.TrimSpace(buildPhotoSummaryContent(photoAnalysis))
		return wrapUserData(fmt.Sprintf(
			"Photo relevance: low for service type '%s'. The image content likely does not match the requested service. Use this photo analysis only as mismatch signal, not as evidence that intake requirements are complete.\n\nMismatch evidence from photo analysis:\n%s",
			serviceType,
			details,
		))
	}
	return buildPhotoSummary(photoAnalysis)
}

func isPhotoAnalysisLikelyIrrelevant(photoAnalysis *repository.PhotoAnalysis) bool {
	if photoAnalysis == nil {
		return false
	}
	// Prefer explicit structured signal from the LLM when available.
	if photoAnalysis.IsRelevant != nil {
		return !*photoAnalysis.IsRelevant
	}
	combined := strings.ToLower(strings.TrimSpace(photoAnalysis.Summary + " " + strings.Join(photoAnalysis.Discrepancies, " ")))
	if containsAny(combined, []string{
		"niet de betreffende",
		"komt niet overeen",
		"niet relevant",
		"mismatch",
		"onverwant",
		"does not match",
		"not relevant",
		"niets te maken met",
		"geen betrekking",
		"onduidelijk",
		"niet zichtbaar",
		"niet herkenbaar",
		"verkeerde",
		"andere ruimte",
		"andere situatie",
		"irrelevant",
		"unrelated",
		"wrong room",
		"wrong area",
		"unclear image",
		" blurry",
		"blurry ",
		"too dark",
		"te donker",
	}) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(photoAnalysis.ConfidenceLevel), "low") && len(photoAnalysis.Discrepancies) > 0
}
