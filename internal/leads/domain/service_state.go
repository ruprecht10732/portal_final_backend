// Package domain provides core business rules for the leads bounded context.
package domain

import "strings"

const (
	LeadStatusNew                  = "New"
	LeadStatusPending              = "Pending"
	LeadStatusInProgress           = "In_Progress"
	LeadStatusAttemptedContact     = "Attempted_Contact"
	LeadStatusAppointmentScheduled = "Appointment_Scheduled"
	LeadStatusNeedsRescheduling    = "Needs_Rescheduling"
	LeadStatusDisqualified         = "Disqualified"
)

// terminalPipelineStages are pipeline stages where the workflow is complete.
var terminalPipelineStages = map[string]bool{
	PipelineStageCompleted: true,
	PipelineStageLost:      true,
}

// IsTerminal returns true if the service is in a terminal state based on
// pipeline stage. A terminal service must not be processed by any AI agent
// (Gatekeeper, Estimator, Dispatcher, Photo Analyzer).
func IsTerminal(status, pipelineStage string) bool {
	_ = status
	return terminalPipelineStages[pipelineStage]
}

// IsTerminalPipelineStage returns true if the pipeline stage alone is terminal.
func IsTerminalPipelineStage(stage string) bool {
	return terminalPipelineStages[stage]
}

// ValidateStateCombination checks that a (status, pipelineStage) pair is
// not contradictory. Returns a non-empty reason string when the combination
// is invalid.
func ValidateStateCombination(status, pipelineStage string) string {
	if status == LeadStatusDisqualified && pipelineStage != PipelineStageLost {
		return "Disqualified status requires Lost pipeline stage"
	}
	if pipelineStage == PipelineStageLost && status != LeadStatusDisqualified {
		return "Lost pipeline stage requires Disqualified status"
	}
	if pipelineStage == PipelineStageProposal && status != LeadStatusPending {
		return "Proposal pipeline stage requires Pending status"
	}
	if pipelineStage == PipelineStageFulfillment && status != LeadStatusPending && status != LeadStatusInProgress {
		return "Fulfillment pipeline stage requires Pending or In_Progress status"
	}
	return ""
}

// GetGoogleConversionName maps lead-service events to Google Ads conversion names.
// This is the canonical mapping for the new (no-legacy) stage/status model.
//
// Expected inputs come from RAC_lead_service_events where:
// - event_type is one of: status_changed, pipeline_stage_changed, service_created, visit_completed
// - status/pipeline_stage are snapshots of the service at event time.
func GetGoogleConversionName(eventType string, status *string, pipelineStage *string) string {
	// Normalize inputs
	et := strings.ToLower(strings.TrimSpace(eventType))
	s := ""
	if status != nil {
		s = strings.ToLower(strings.TrimSpace(*status))
	}
	p := ""
	if pipelineStage != nil {
		p = strings.ToLower(strings.TrimSpace(*pipelineStage))
	}

	// 1) Appointment booked (status-driven)
	if et == "status_changed" {
		if s == "appointment_scheduled" || s == "scheduled" {
			return "Appointment_Scheduled"
		}
	}

	// 2) Visit completed (explicit event-driven)
	if et == "visit_completed" {
		return "Visit_Completed"
	}

	// 3) Legacy: survey_completed status → Visit_Completed
	if s == "survey_completed" {
		return "Visit_Completed"
	}

	// 4) Stage-driven conversions (event-type gated)
	if et == "pipeline_stage_changed" {
		switch p {
		case "estimation":
			return "Lead_Qualified"
		case "proposal":
			return "Quote_Sent"
		case "fulfillment":
			return "Deal_Won"
		}
	}

	// 5) Legacy stage fallbacks (no event_type gate for pre-migration events)
	switch p {
	case "ready_for_estimator":
		return "Lead_Qualified"
	case "quote_sent":
		return "Quote_Sent"
	case "partner_assigned", "partner_matching", "ready_for_partner":
		return "Deal_Won"
	}

	// 6) Legacy: quote_accepted status → Deal_Won
	if s == "quote_accepted" {
		return "Deal_Won"
	}

	return ""
}
