// Package domain provides core business rules for the leads bounded context.
package domain

// terminalStatuses are service statuses where no further agent actions should occur.
var terminalStatuses = map[string]bool{
	"Closed":   true,
	"Bad_Lead": true,
	"Surveyed": true,
}

// terminalPipelineStages are pipeline stages where the workflow is complete.
var terminalPipelineStages = map[string]bool{
	"Completed": true,
	"Lost":      true,
}

// IsTerminal returns true if the service is in a terminal state based on
// EITHER status or pipeline stage. A terminal service must not be processed
// by any AI agent (Gatekeeper, Estimator, Dispatcher, Photo Analyzer).
func IsTerminal(status, pipelineStage string) bool {
	return terminalStatuses[status] || terminalPipelineStages[pipelineStage]
}

// IsTerminalStatus returns true if the status alone is terminal.
func IsTerminalStatus(status string) bool {
	return terminalStatuses[status]
}

// IsTerminalPipelineStage returns true if the pipeline stage alone is terminal.
func IsTerminalPipelineStage(stage string) bool {
	return terminalPipelineStages[stage]
}

// ValidateStateCombination checks that a (status, pipelineStage) pair is
// not contradictory. Returns a non-empty reason string when the combination
// is invalid.
func ValidateStateCombination(status, pipelineStage string) string {
	// Bad_Lead status must have Lost pipeline stage (or Triage/Manual_Intervention during transition)
	if status == "Bad_Lead" && pipelineStage != "Lost" && pipelineStage != "Triage" && pipelineStage != "Manual_Intervention" {
		return "Bad_Lead status requires Lost pipeline stage"
	}
	return ""
}
