// Package domain provides core business rules for the leads bounded context.
package domain

const (
	LeadStatusNew                  = "New"
	LeadStatusAttemptedContact     = "Attempted_Contact"
	LeadStatusAppointmentScheduled = "Appointment_Scheduled"
	LeadStatusSurveyCompleted      = "Survey_Completed"
	LeadStatusQuoteDraft           = "Quote_Draft"
	LeadStatusQuoteSent            = "Quote_Sent"
	LeadStatusQuoteAccepted        = "Quote_Accepted"
	LeadStatusPartnerAssigned      = "Partner_Assigned"
	LeadStatusNeedsRescheduling    = "Needs_Rescheduling"
	LeadStatusCompleted            = "Completed"
	LeadStatusLost                 = "Lost"
	LeadStatusDisqualified         = "Disqualified"
)

// terminalStatuses are service statuses where no further agent actions should occur.
var terminalStatuses = map[string]bool{
	LeadStatusCompleted:    true,
	LeadStatusLost:         true,
	LeadStatusDisqualified: true,
}

// terminalPipelineStages are pipeline stages where the workflow is complete.
var terminalPipelineStages = map[string]bool{
	PipelineStageCompleted: true,
	PipelineStageLost:      true,
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
	if status == LeadStatusDisqualified && pipelineStage != PipelineStageLost && pipelineStage != PipelineStageTriage && pipelineStage != PipelineStageManualIntervention {
		return "Disqualified status requires Lost pipeline stage"
	}

	if status == LeadStatusLost && pipelineStage != PipelineStageLost {
		return "Lost status requires Lost pipeline stage"
	}
	return ""
}
