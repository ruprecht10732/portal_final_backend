package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
)

type estimatorReasoningMode string

const (
	reasoningModeFast       estimatorReasoningMode = "fast"
	reasoningModeBalanced   estimatorReasoningMode = "balanced"
	reasoningModeDeliberate estimatorReasoningMode = "deliberate"
)

type estimatorCouncilAdvice struct {
	RecommendedStage string
	Warnings         []string
	Signals          []string
}

func chooseEstimatorReasoningMode(settings ports.OrganizationAISettings, analysis *repository.AIAnalysis, photo *repository.PhotoAnalysis) estimatorReasoningMode {
	if !settings.AIAdaptiveReasoning {
		return reasoningModeBalanced
	}
	if analysis == nil {
		return reasoningModeBalanced
	}

	missingCount := len(analysis.MissingInformation)
	confidence := 0.0
	if analysis.CompositeConfidence != nil {
		confidence = *analysis.CompositeConfidence
	}

	if missingCount >= 3 || confidence < 0.35 {
		return reasoningModeDeliberate
	}
	if missingCount == 0 && confidence >= 0.75 {
		if photo == nil || strings.EqualFold(strings.TrimSpace(photo.ConfidenceLevel), "High") {
			return reasoningModeFast
		}
	}
	return reasoningModeBalanced
}

func isInvestigativeMode(mode estimatorReasoningMode) bool {
	return mode == reasoningModeDeliberate
}

func buildExperienceMemorySection(memories []repository.AIDecisionMemory) string {
	if len(memories) == 0 {
		return "=== EXPERIENCE MEMORY ===\nNo prior decision memories found for this service type."
	}

	var sb strings.Builder
	sb.WriteString("=== EXPERIENCE MEMORY ===\n")
	for i, m := range memories {
		if i >= 6 {
			break
		}
		conf := "n/a"
		if m.Confidence != nil {
			conf = fmt.Sprintf("%.2f", *m.Confidence)
		}
		sb.WriteString(fmt.Sprintf("- [%s/%s] confidence=%s | context=%s | action=%s\n",
			compactText(m.DecisionType, 24),
			compactText(m.Outcome, 24),
			conf,
			compactText(m.ContextSummary, 140),
			compactText(m.ActionSummary, 140),
		))
	}
	return strings.TrimSpace(sb.String())
}

func buildHumanFeedbackMemorySection(feedbackItems []repository.HumanFeedback) string {
	if len(feedbackItems) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== HUMAN FEEDBACK MEMORY ===\n")
	for i, item := range feedbackItems {
		if i >= 6 {
			break
		}
		delta := "n/a"
		if item.DeltaPercentage != nil {
			delta = fmt.Sprintf("%.2f%%", *item.DeltaPercentage)
		}
		aiJSON, _ := json.Marshal(item.AIValue)
		humanJSON, _ := json.Marshal(item.HumanValue)

		sb.WriteString(fmt.Sprintf("- field=%s | delta=%s | ai=%s | human=%s\n",
			compactText(item.FieldChanged, 48),
			delta,
			compactText(string(aiJSON), 140),
			compactText(string(humanJSON), 140),
		))
	}
	return strings.TrimSpace(sb.String())
}

func buildPricingIntelligenceSection(report *ports.PricingIntelligenceReport) string {
	if report == nil {
		return ""
	}
	if len(report.Aggregates) == 0 && len(report.RecentOutcomes) == 0 && len(report.RecentCorrections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== PRICING INTELLIGENCE ===\n")
	if strings.TrimSpace(report.RegionPrefix) != "" {
		sb.WriteString(fmt.Sprintf("Filtered region: %s\n", report.RegionPrefix))
	}
	appendPricingAggregateLines(&sb, report.Aggregates)
	appendPricingOutcomeLines(&sb, report.RecentOutcomes)
	appendPricingCorrectionLines(&sb, report.RecentCorrections)
	return strings.TrimSpace(sb.String())
}

func appendPricingAggregateLines(sb *strings.Builder, aggregates []ports.PricingIntelligenceAggregate) {
	for i, aggregate := range aggregates {
		if i >= 4 {
			break
		}
		line := fmt.Sprintf("- aggregate region=%s | band=%s | samples=%d | accepted=%d | rejected=%d | conversion=%.1f%% | avgQuote=%s",
			compactText(fallbackRegion(aggregate.RegionPrefix), 12),
			compactText(aggregate.PriceBand, 24),
			aggregate.SampleCount,
			aggregate.AcceptedCount,
			aggregate.RejectedCount,
			aggregate.ConversionRate,
			formatEuroCents(aggregate.AverageQuotedCents),
		)
		if aggregate.AverageOutcomeCents != nil {
			line += fmt.Sprintf(" | avgOutcome=%s", formatEuroCents(*aggregate.AverageOutcomeCents))
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

func appendPricingOutcomeLines(sb *strings.Builder, outcomes []ports.PricingIntelligenceOutcomeRecord) {
	for i, outcome := range outcomes {
		if i >= 3 {
			break
		}
		line := fmt.Sprintf("- outcome=%s | region=%s | band=%s | total=%s",
			compactText(outcome.OutcomeType, 16),
			compactText(fallbackRegion(outcome.RegionPrefix), 12),
			compactText(outcome.PriceBand, 24),
			formatOptionalEuroCents(outcome.FinalTotalCents),
		)
		if outcome.Reason != nil {
			line += " | reason=" + compactText(*outcome.Reason, 80)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

func appendPricingCorrectionLines(sb *strings.Builder, corrections []ports.PricingIntelligenceCorrectionRecord) {
	for i, correction := range corrections {
		if i >= 4 {
			break
		}
		line := fmt.Sprintf("- correction field=%s | region=%s | band=%s | delta=%s",
			compactText(correction.FieldName, 48),
			compactText(fallbackRegion(correction.RegionPrefix), 12),
			compactText(correction.PriceBand, 24),
			formatOptionalPercentage(correction.DeltaPercentage),
		)
		if correction.Reason != nil {
			line += " | reason=" + compactText(*correction.Reason, 60)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

func buildCouncilSection(advice estimatorCouncilAdvice) string {
	if advice.RecommendedStage == "" && len(advice.Warnings) == 0 && len(advice.Signals) == 0 {
		return ""
	}
	warnings := "none"
	if len(advice.Warnings) > 0 {
		warnings = strings.Join(advice.Warnings, "; ")
	}
	signals := "none"
	if len(advice.Signals) > 0 {
		signals = strings.Join(advice.Signals, "; ")
	}
	return fmt.Sprintf("=== MULTI-AGENT COUNCIL ===\nRecommended stage: %s\nSignals: %s\nWarnings: %s", advice.RecommendedStage, signals, warnings)
}

func runEstimatorCouncil(analysis *repository.AIAnalysis, photo *repository.PhotoAnalysis, notes []repository.LeadNote) estimatorCouncilAdvice {
	advice := estimatorCouncilAdvice{RecommendedStage: "Estimation"}
	if analysis == nil {
		return advice
	}

	applyAnalysisSignals(&advice, analysis)
	applyPhotoSignals(&advice, photo)

	noteSignals := countUncertainNoteSignals(notes)
	if noteSignals > 0 {
		advice.Signals = append(advice.Signals, fmt.Sprintf("uncertain_note_markers=%d", noteSignals))
	}

	sort.Strings(advice.Warnings)
	sort.Strings(advice.Signals)
	return advice
}

func applyAnalysisSignals(advice *estimatorCouncilAdvice, analysis *repository.AIAnalysis) {
	if strings.EqualFold(strings.TrimSpace(analysis.RecommendedAction), "RequestInfo") {
		advice.RecommendedStage = "Nurturing"
		advice.Warnings = append(advice.Warnings, "Gatekeeper requested extra intake information")
	}
	if len(analysis.MissingInformation) > 0 {
		advice.Signals = append(advice.Signals, fmt.Sprintf("missing_info_count=%d", len(analysis.MissingInformation)))
	}
	if analysis.CompositeConfidence != nil {
		advice.Signals = append(advice.Signals, fmt.Sprintf("analysis_confidence=%.2f", *analysis.CompositeConfidence))
		if *analysis.CompositeConfidence < 0.45 {
			advice.RecommendedStage = "Nurturing"
			advice.Warnings = append(advice.Warnings, "Confidence below estimation guardrail")
		}
	}
}

func applyPhotoSignals(advice *estimatorCouncilAdvice, photo *repository.PhotoAnalysis) {
	if photo != nil {
		relevance := strings.TrimSpace(photo.ConfidenceLevel)
		if relevance == "" {
			relevance = "Unknown"
		}
		advice.Signals = append(advice.Signals, "photo_confidence="+relevance)
		if strings.EqualFold(relevance, "Low") {
			advice.Warnings = append(advice.Warnings, "Photo relevance is low or mismatched")
		}
	}
}

func countUncertainNoteSignals(notes []repository.LeadNote) int {
	noteSignals := 0
	for _, n := range notes {
		body := strings.ToLower(strings.TrimSpace(n.Body))
		if strings.Contains(body, "weet niet") || strings.Contains(body, "onbekend") || strings.Contains(body, "nog meten") {
			noteSignals++
		}
	}
	return noteSignals
}

func compactText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 4 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func formatEuroCents(value int64) string {
	return fmt.Sprintf("EUR %.2f", float64(value)/100)
}

func formatOptionalEuroCents(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return formatEuroCents(*value)
}

func formatOptionalPercentage(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", *value)
}

func fallbackRegion(region string) string {
	if strings.TrimSpace(region) == "" {
		return "all"
	}
	return region
}
