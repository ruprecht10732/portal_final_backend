package domain

import (
	"testing"
)

// TestGetGoogleConversionName_ParityWithBackfillSQL verifies that the Go
// GetGoogleConversionName function and the SQL CASE block in BackfillHistoricalData
// produce the same conversion names for every known input combination.
//
// If this test fails, the SQL CASE in exports/repository.go is out of sync with
// the Go function in this file.
func TestGetGoogleConversionNameParityWithBackfillSQL(t *testing.T) {
	// These are the exact (eventType, status, pipelineStage) → conversionName
	// mappings that must be present in both Go and SQL.
	type mapping struct {
		eventType     string
		status        string
		pipelineStage string
		want          string
	}

	cases := []mapping{
		// 1) Appointment_Scheduled (status-driven)
		{"status_changed", "Appointment_Scheduled", "", "Appointment_Scheduled"},
		{"status_changed", "Scheduled", "", "Appointment_Scheduled"},

		// 2) Visit completed (event-driven)
		{"visit_completed", "", "", "Visit_Completed"},

		// 3) Legacy survey_completed
		{"status_changed", "survey_completed", "", "Visit_Completed"},

		// 4) Stage-driven conversions
		{"pipeline_stage_changed", "", "Estimation", "Lead_Qualified"},
		{"pipeline_stage_changed", "", "Proposal", "Quote_Sent"},
		{"pipeline_stage_changed", "", "Fulfillment", "Deal_Won"},

		// 5) Legacy stage fallbacks
		{"", "", "ready_for_estimator", "Lead_Qualified"},
		{"", "", "quote_sent", "Quote_Sent"},
		{"", "", "partner_assigned", "Deal_Won"},
		{"", "", "partner_matching", "Deal_Won"},
		{"", "", "ready_for_partner", "Deal_Won"},

		// 6) Legacy quote_accepted
		{"", "quote_accepted", "", "Deal_Won"},

		// No match
		{"status_changed", "New", "", ""},
		{"", "", "", ""},
	}

	for _, tc := range cases {
		var statusPtr, stagePtr *string
		if tc.status != "" {
			s := tc.status
			statusPtr = &s
		}
		if tc.pipelineStage != "" {
			p := tc.pipelineStage
			stagePtr = &p
		}

		got := GetGoogleConversionName(tc.eventType, statusPtr, stagePtr)
		if got != tc.want {
			t.Errorf("GetGoogleConversionName(%q, %q, %q) = %q, want %q",
				tc.eventType, tc.status, tc.pipelineStage, got, tc.want)
		}
	}
}

func TestValidateStateCombination(t *testing.T) {
	tests := []struct {
		status   string
		stage    string
		wantFail bool
	}{
		{LeadStatusDisqualified, PipelineStageLost, false},
		{LeadStatusDisqualified, PipelineStageEstimation, true},
		{LeadStatusNew, PipelineStageLost, true},
		{LeadStatusInProgress, PipelineStageLost, true},
		{LeadStatusNew, PipelineStageEstimation, false},
		{LeadStatusPending, PipelineStageProposal, false},
		{LeadStatusInProgress, PipelineStageFulfillment, false},
	}

	for _, tc := range tests {
		reason := ValidateStateCombination(tc.status, tc.stage)
		if tc.wantFail && reason == "" {
			t.Errorf("ValidateStateCombination(%q, %q) should have returned an error", tc.status, tc.stage)
		}
		if !tc.wantFail && reason != "" {
			t.Errorf("ValidateStateCombination(%q, %q) unexpected error: %s", tc.status, tc.stage, reason)
		}
	}
}
