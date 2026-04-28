package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

func isEquivalentRecentAnalysis(current repository.AIAnalysis, candidate normalizedAnalysisInput) bool {
	if time.Since(current.CreatedAt) > recentEquivalentAnalysisTTL {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.UrgencyLevel), strings.TrimSpace(candidate.UrgencyLevel)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.LeadQuality), strings.TrimSpace(candidate.LeadQuality)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.RecommendedAction), strings.TrimSpace(candidate.RecommendedAction)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.PreferredContactChannel), strings.TrimSpace(candidate.Channel)) {
		return false
	}
	if strings.TrimSpace(normalizeSuggestedContactMessage(current.SuggestedContactMessage)) != strings.TrimSpace(normalizeSuggestedContactMessage(candidate.SuggestedMessage)) {
		return false
	}
	return equalNormalizedStringSlices(current.MissingInformation, candidate.MissingInformation) &&
		equalNormalizedStringSlices(current.ResolvedInformation, candidate.ResolvedInformation) &&
		equalNormalizedFactMaps(current.ExtractedFacts, candidate.ExtractedFacts)
}

func equalNormalizedStringSlices(current []string, candidate []string) bool {
	currentNormalized := normalizeMissingInformation(current)
	candidateNormalized := normalizeMissingInformation(candidate)
	if len(currentNormalized) != len(candidateNormalized) {
		return false
	}
	for i := range currentNormalized {
		if !strings.EqualFold(currentNormalized[i], candidateNormalized[i]) {
			return false
		}
	}
	return true
}

func equalNormalizedFactMaps(current map[string]string, candidate map[string]string) bool {
	currentNormalized := normalizeExtractedFacts(current)
	candidateNormalized := normalizeExtractedFacts(candidate)
	if len(currentNormalized) != len(candidateNormalized) {
		return false
	}
	for key, value := range currentNormalized {
		if !strings.EqualFold(strings.TrimSpace(candidateNormalized[key]), strings.TrimSpace(value)) {
			return false
		}
	}
	return true
}

func parseLeadServiceID(value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.UUID{}, fmt.Errorf("missing lead service ID")
	}
	return parseUUID(value, invalidLeadServiceIDMessage)
}

func resolveSaveAnalysisContext(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (uuid.UUID, uuid.UUID, uuid.UUID, SaveAnalysisOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		log.Printf("handleSaveAnalysis: FAILED - invalid leadID: %s", input.LeadID)
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadServiceID, err := parseLeadServiceID(input.LeadServiceID)
	if err != nil {
		message := err.Error()
		if err.Error() == invalidLeadServiceIDMessage {
			message = invalidLeadServiceIDMessage
		}
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, SaveAnalysisOutput{Success: false, Message: message}, err
	}

	if out, err := rejectTerminalAnalysisService(ctx, deps, tenantID, leadServiceID); err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, out, err
	}

	return leadID, tenantID, leadServiceID, SaveAnalysisOutput{}, nil
}

func rejectTerminalAnalysisService(ctx tool.Context, deps *ToolDependencies, tenantID, leadServiceID uuid.UUID) (SaveAnalysisOutput, error) {
	// Terminal check: refuse to save analysis for terminal services.
	svc, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleSaveAnalysis: REJECTED - service %s is in terminal state (status=%s, stage=%s)", leadServiceID, svc.Status, svc.PipelineStage)
		return SaveAnalysisOutput{Success: false, Message: "Cannot save analysis for a service in terminal state"}, fmt.Errorf("service %s is terminal", leadServiceID)
	}
	return SaveAnalysisOutput{}, nil
}

type normalizedAnalysisInput struct {
	UrgencyLevel        string
	UrgencyReason       *string
	LeadQuality         string
	RecommendedAction   string
	Channel             string
	MissingInformation  []string
	ResolvedInformation []string
	ExtractedFacts      map[string]string
	SuggestedMessage    string
	Summary             string
	CompositeConfidence float64
	ConfidenceBreakdown map[string]float64
	RiskFlags           []string
	Metadata            map[string]any
}

type trustedAnalysisContext struct {
	service       *repository.LeadService
	priorAnalysis *repository.AIAnalysis
	visitReport   *repository.AppointmentVisitReport
	attachments   []repository.Attachment
}

type analysisFactCollector struct {
	resolved map[string]string
	facts    map[string]string
}

type analysisPreferences struct {
	Budget       string `json:"budget"`
	Timeframe    string `json:"timeframe"`
	Availability string `json:"availability"`
	ExtraNotes   string `json:"extraNotes"`
}

func loadTrustedAnalysisContext(ctx context.Context, deps *ToolDependencies, tenantID, leadServiceID uuid.UUID) trustedAnalysisContext {
	trusted := trustedAnalysisContext{}
	if service, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID); err == nil {
		trusted.service = &service
	}
	if analysis, err := deps.Repo.GetLatestAIAnalysis(ctx, leadServiceID, tenantID); err == nil {
		trusted.priorAnalysis = &analysis
	}
	if report, err := deps.Repo.GetLatestAppointmentVisitReportByService(ctx, leadServiceID, tenantID); err == nil {
		trusted.visitReport = report
	}
	if attachments, err := deps.Repo.ListAttachmentsByService(ctx, leadServiceID, tenantID); err == nil {
		trusted.attachments = attachments
	}
	return trusted
}

func populateAnalysisFacts(ctx context.Context, deps *ToolDependencies, lead repository.Lead, tenantID, leadServiceID uuid.UUID, resolvedInformation []string, extractedFacts map[string]string) ([]string, map[string]string) {
	trusted := loadTrustedAnalysisContext(ctx, deps, tenantID, leadServiceID)
	collector := newAnalysisFactCollector(resolvedInformation, extractedFacts)
	mergeLeadFacts(&collector, lead)
	mergeServiceFacts(&collector, trusted.service)
	mergeVisitReportFacts(&collector, trusted.visitReport)
	mergeAttachmentFacts(&collector, trusted.attachments)
	mergePriorAnalysisFacts(&collector, trusted.priorAnalysis)
	return collector.resolvedValues(), collector.extractedFactMap()
}

func newAnalysisFactCollector(resolvedInformation []string, extractedFacts map[string]string) analysisFactCollector {
	collector := analysisFactCollector{
		resolved: map[string]string{},
		facts:    map[string]string{},
	}
	for _, value := range normalizeMissingInformation(resolvedInformation) {
		collector.addResolved(value)
	}
	for key, value := range normalizeExtractedFacts(extractedFacts) {
		collector.addFact(key, value)
	}
	return collector
}

func (c *analysisFactCollector) addResolved(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	normalized := strings.ToLower(trimmed)
	if _, exists := c.resolved[normalized]; exists {
		return
	}
	c.resolved[normalized] = trimmed
}

func (c *analysisFactCollector) addFact(key string, value string) {
	trimmedKey := strings.TrimSpace(key)
	trimmedValue := strings.TrimSpace(value)
	if trimmedKey == "" || trimmedValue == "" {
		return
	}
	if _, exists := c.facts[trimmedKey]; exists {
		return
	}
	c.facts[trimmedKey] = trimmedValue
}

func (c *analysisFactCollector) resolvedValues() []string {
	values := make([]string, 0, len(c.resolved))
	for _, value := range c.resolved {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
	return values
}

func (c *analysisFactCollector) extractedFactMap() map[string]string {
	result := make(map[string]string, len(c.facts))
	for key, value := range c.facts {
		result[key] = value
	}
	return result
}

func mergePriorAnalysisFacts(collector *analysisFactCollector, priorAnalysis *repository.AIAnalysis) {
	if priorAnalysis == nil {
		return
	}
	for _, value := range priorAnalysis.ResolvedInformation {
		collector.addResolved(value)
	}
	for key, value := range priorAnalysis.ExtractedFacts {
		collector.addFact(key, value)
	}
}

func mergeLeadFacts(collector *analysisFactCollector, lead repository.Lead) {
	if lead.EnergyClass != nil {
		collector.addFact("energy_class", *lead.EnergyClass)
	}
	if lead.EnergyBouwjaar != nil {
		collector.addFact("build_year", strconv.Itoa(*lead.EnergyBouwjaar))
	}
	if lead.EnergyGebouwtype != nil {
		collector.addFact("building_type", *lead.EnergyGebouwtype)
	}
}

func mergeServiceFacts(collector *analysisFactCollector, service *repository.LeadService) {
	if service == nil {
		return
	}
	collector.addFact("service_type", service.ServiceType)
	collector.addFact("consumer_note", trustedOptionalString(service.ConsumerNote))
	prefs := parseAnalysisPreferences(service.CustomerPreferences)
	addPreferenceFact(collector, "budget", prefs.Budget, "Budget gedeeld: %s")
	addPreferenceFact(collector, "timeframe", prefs.Timeframe, "Gewenste termijn: %s")
	addPreferenceFact(collector, "availability", prefs.Availability, "Beschikbaarheid gedeeld: %s")
	collector.addFact("preference_notes", prefs.ExtraNotes)
}

func parseAnalysisPreferences(raw json.RawMessage) analysisPreferences {
	if len(raw) == 0 {
		return analysisPreferences{}
	}
	var prefs analysisPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return analysisPreferences{}
	}
	prefs.Budget = strings.TrimSpace(prefs.Budget)
	prefs.Timeframe = strings.TrimSpace(prefs.Timeframe)
	prefs.Availability = strings.TrimSpace(prefs.Availability)
	prefs.ExtraNotes = strings.TrimSpace(prefs.ExtraNotes)
	return prefs
}

func addPreferenceFact(collector *analysisFactCollector, key string, value string, resolvedTemplate string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	collector.addFact(key, value)
	collector.addResolved(fmt.Sprintf(resolvedTemplate, value))
}

func trustedOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || strings.EqualFold(trimmed, valueNotProvided) {
		return ""
	}
	return trimmed
}

func mergeVisitReportFacts(collector *analysisFactCollector, visitReport *repository.AppointmentVisitReport) {
	if visitReport == nil {
		return
	}
	if measurements := trustedOptionalString(visitReport.Measurements); measurements != "" {
		collector.addFact("visit_report_measurements", measurements)
		collector.addResolved("Ingemeten tijdens afspraak: " + measurements)
	}
	collector.addFact("visit_report_access_difficulty", trustedOptionalString(visitReport.AccessDifficulty))
	collector.addFact("visit_report_notes", trustedOptionalString(visitReport.Notes))
}

func mergeAttachmentFacts(collector *analysisFactCollector, attachments []repository.Attachment) {
	if len(attachments) == 0 {
		return
	}
	documentNames := make([]string, 0, len(attachments))
	requiresDocumentReview := false
	for _, attachment := range attachments {
		_, isNonImageDocument, requiresReview := classifyAttachment(attachment)
		if isNonImageDocument {
			documentNames = append(documentNames, strings.TrimSpace(attachment.FileName))
		}
		if requiresReview {
			requiresDocumentReview = true
		}
	}
	if len(documentNames) > 0 {
		sort.Strings(documentNames)
		collector.addFact("attachment_documents", strings.Join(documentNames, ", "))
	}
	if requiresDocumentReview {
		collector.addFact("document_review_required", "true")
		collector.addResolved("Klant heeft document(en) geüpload voor handmatige controle")
	}
}

func normalizeAnalysisInput(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput, lead repository.Lead, tenantID, leadServiceID uuid.UUID) (normalizedAnalysisInput, error) {
	urgencyLevel, err := normalizeUrgencyLevel(input.UrgencyLevel)
	if err != nil {
		return normalizedAnalysisInput{}, err
	}

	var urgencyReason *string
	trimmedUrgencyReason := strings.TrimSpace(input.UrgencyReason)
	if trimmedUrgencyReason != "" {
		urgencyReason = &trimmedUrgencyReason
	}

	channel, err := resolvePreferredChannel(input.PreferredContactChannel, lead)
	if err != nil {
		return normalizedAnalysisInput{}, fmt.Errorf("invalid preferred contact channel")
	}

	leadQuality := normalizeLeadQuality(input.LeadQuality)
	recommendedAction := normalizeRecommendedAction(input.RecommendedAction)
	log.Printf("handleSaveAnalysis: normalized recommendedAction '%s' -> '%s'", input.RecommendedAction, recommendedAction)

	missingInformation := normalizeMissingInformation(input.MissingInformation)

	// Guard: LLMs sometimes produce a contradiction where missing fields are listed
	// but the recommended action is not "RequestInfo". Force the correct action so
	// the domain invariant check (ValidateAnalysisStageTransition) is never bypassed.
	if len(missingInformation) > 0 && recommendedAction != "RequestInfo" {
		log.Printf("normalizeAnalysisInput: auto-correcting contradictory recommendedAction '%s' -> 'RequestInfo' (missingInformation non-empty, count=%d)", recommendedAction, len(missingInformation))
		recommendedAction = "RequestInfo"
	}

	resolvedInformation := normalizeMissingInformation(input.ResolvedInformation)
	extractedFacts := normalizeExtractedFacts(input.ExtractedFacts)
	resolvedInformation, extractedFacts = populateAnalysisFacts(ctx, deps, lead, tenantID, leadServiceID, resolvedInformation, extractedFacts)
	normalizedMessage := normalizeSuggestedContactMessage(input.SuggestedContactMessage)
	analysisSummary := strings.TrimSpace(input.Summary)
	if analysisSummary == "" {
		analysisSummary = fmt.Sprintf("AI analyse voltooid: %s urgentie, aanbevolen actie: %s", urgencyLevel, recommendedAction)
	}

	meta := repository.AIAnalysisMetadata{
		RunID:             deps.GetRunID(),
		UrgencyLevel:      urgencyLevel,
		RecommendedAction: recommendedAction,
		LeadQuality:       leadQuality,
	}
	if normalizedMessage != "" {
		meta.SuggestedContactMessage = normalizedMessage
		meta.PreferredContactChannel = string(channel)
	}
	if len(missingInformation) > 0 {
		meta.MissingInformation = missingInformation
	}
	if len(resolvedInformation) > 0 {
		meta.ResolvedInformation = resolvedInformation
	}
	if len(extractedFacts) > 0 {
		meta.ExtractedFacts = extractedFacts
	}

	confidence := calculateAnalysisConfidence(lead, leadQuality, recommendedAction, missingInformation)
	meta.CompositeConfidence = confidence.Score
	meta.ConfidenceBreakdown = confidence.Breakdown
	meta.RiskFlags = confidence.RiskFlags

	return normalizedAnalysisInput{
		UrgencyLevel:        urgencyLevel,
		UrgencyReason:       urgencyReason,
		LeadQuality:         leadQuality,
		RecommendedAction:   recommendedAction,
		Channel:             channel,
		MissingInformation:  missingInformation,
		ResolvedInformation: resolvedInformation,
		ExtractedFacts:      extractedFacts,
		SuggestedMessage:    normalizedMessage,
		Summary:             analysisSummary,
		CompositeConfidence: confidence.Score,
		ConfidenceBreakdown: confidence.Breakdown,
		RiskFlags:           confidence.RiskFlags,
		Metadata:            meta.ToMap(),
	}, nil
}

func shouldSkipEquivalentRecentAnalysis(ctx tool.Context, deps *ToolDependencies, leadServiceID, tenantID uuid.UUID, normalized normalizedAnalysisInput) bool {
	latest, latestErr := deps.Repo.GetLatestAIAnalysis(ctx, leadServiceID, tenantID)
	if latestErr != nil {
		return false
	}
	return isEquivalentRecentAnalysis(latest, normalized)
}

func handleSaveAnalysis(ctx tool.Context, deps *ToolDependencies, input SaveAnalysisInput) (SaveAnalysisOutput, error) {
	log.Printf("handleSaveAnalysis: CALLED with leadID=%s serviceID=%s urgency=%s action=%s",
		input.LeadID, input.LeadServiceID, input.UrgencyLevel, input.RecommendedAction)

	leadID, tenantID, leadServiceID, contextOut, err := resolveSaveAnalysisContext(ctx, deps, input)
	if err != nil {
		return contextOut, err
	}

	lead, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: leadNotFoundMessage}, err
	}

	normalized, err := normalizeAnalysisInput(ctx, deps, input, lead, tenantID, leadServiceID)
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	if shouldSkipEquivalentRecentAnalysis(ctx, deps, leadServiceID, tenantID, normalized) {
		deps.SetLastAnalysisMetadata(normalized.Metadata)
		deps.MarkSaveAnalysisCalled()
		log.Printf("handleSaveAnalysis: skipped duplicate-equivalent analysis for lead=%s service=%s", leadID, leadServiceID)
		return SaveAnalysisOutput{Success: true, Message: "Analysis unchanged; duplicate save skipped"}, nil
	}

	_, err = deps.Repo.CreateAIAnalysis(context.Background(), repository.CreateAIAnalysisParams{
		LeadID:                  leadID,
		OrganizationID:          tenantID,
		LeadServiceID:           leadServiceID,
		UrgencyLevel:            normalized.UrgencyLevel,
		UrgencyReason:           normalized.UrgencyReason,
		LeadQuality:             normalized.LeadQuality,
		RecommendedAction:       normalized.RecommendedAction,
		MissingInformation:      normalized.MissingInformation,
		ResolvedInformation:     normalized.ResolvedInformation,
		ExtractedFacts:          normalized.ExtractedFacts,
		CompositeConfidence:     &normalized.CompositeConfidence,
		ConfidenceBreakdown:     normalized.ConfidenceBreakdown,
		RiskFlags:               normalized.RiskFlags,
		PreferredContactChannel: normalized.Channel,
		SuggestedContactMessage: normalized.SuggestedMessage,
		Summary:                 normalized.Summary,
	})
	if err != nil {
		return SaveAnalysisOutput{Success: false, Message: err.Error()}, err
	}

	actorType, actorName := deps.GetActor()

	// Create comprehensive analysis timeline event for frontend rendering.
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeAI,
		Title:          repository.EventTitleGatekeeperAnalysis,
		Summary:        &normalized.Summary,
		Metadata:       normalized.Metadata,
	})

	// Store analysis metadata for use in stage_change events
	deps.SetLastAnalysisMetadata(normalized.Metadata)
	log.Printf("SaveAnalysis: stored analysis metadata for lead=%s service=%s channel=%s action=%s",
		leadID, leadServiceID, normalized.Channel, normalized.RecommendedAction)

	recalculateAndRecordScore(ctx, deps, leadID, leadServiceID, tenantID, actorType, actorName)

	log.Printf(
		"gatekeeper SaveAnalysis: run=%s leadId=%s serviceId=%s urgency=%s quality=%s action=%s missing=%d confidence=%.2f risk_flags=%d",
		deps.GetRunID(),
		leadID,
		leadServiceID,
		normalized.UrgencyLevel,
		normalized.LeadQuality,
		normalized.RecommendedAction,
		len(input.MissingInformation),
		normalized.CompositeConfidence,
		len(normalized.RiskFlags),
	)

	deps.MarkSaveAnalysisCalled()
	return SaveAnalysisOutput{Success: true, Message: "Analysis saved successfully"}, nil
}

func recalculateAndRecordScore(ctx tool.Context, deps *ToolDependencies, leadID, leadServiceID, tenantID uuid.UUID, actorType, actorName string) {
	if deps.Scorer == nil {
		return
	}
	scoreResult, scoreErr := deps.Scorer.Recalculate(ctx, leadID, &leadServiceID, tenantID, true)
	if scoreErr != nil {
		return
	}
	_ = deps.Repo.UpdateLeadScore(ctx, leadID, tenantID, repository.UpdateLeadScoreParams{
		Score:          &scoreResult.Score,
		ScorePreAI:     &scoreResult.ScorePreAI,
		ScoreFactors:   scoreResult.FactorsJSON,
		ScoreVersion:   &scoreResult.Version,
		ScoreUpdatedAt: scoreResult.UpdatedAt,
	})

	summary := buildLeadScoreSummary(scoreResult)
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeAnalysis,
		Title:          repository.EventTitleLeadScoreUpdated,
		Summary:        &summary,
		Metadata: repository.LeadScoreMetadata{
			RunID:            deps.GetRunID(),
			LeadScore:        scoreResult.Score,
			LeadScorePreAI:   scoreResult.ScorePreAI,
			LeadScoreVersion: scoreResult.Version,
		}.ToMap(),
	})
}

func buildLeadScoreSummary(result *scoring.Result) string {
	return fmt.Sprintf("Leadscore %d (pre-AI %d)", result.Score, result.ScorePreAI)
}
