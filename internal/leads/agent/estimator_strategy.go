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
