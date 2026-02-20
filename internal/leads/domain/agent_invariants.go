package domain

import "strings"

// HasNonEmptyMissingInformation returns true when the list contains at least one
// non-blank entry.
func HasNonEmptyMissingInformation(items []string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

// ValidateAnalysisStageTransition enforces analysis-stage invariants shared by
// gatekeeper, estimator, and reconciliation paths.
//
// Returns a non-empty reason when the transition must be blocked.
func ValidateAnalysisStageTransition(recommendedAction string, missingInformation []string, targetStage string) string {
	if targetStage != PipelineStageEstimation {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(recommendedAction), "RequestInfo") {
		return "Cannot move to Estimation while recommendedAction is RequestInfo"
	}
	if HasNonEmptyMissingInformation(missingInformation) {
		return "Cannot move to Estimation while critical intake information is missing"
	}
	return ""
}
