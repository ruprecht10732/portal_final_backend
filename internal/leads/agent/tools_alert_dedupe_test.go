package agent

import "testing"

const (
	blockedDraftQuoteSummary = "Council blokkeert conceptofferte: intake is nog onvolledig."
	firstAlertExpected       = "expected first alert emission to be recorded"
)

func TestMarkAlertEmittedDeduplicatesSameAlert(t *testing.T) {
	deps := &ToolDependencies{}

	if !deps.MarkAlertEmitted("council_draft_quote", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal(firstAlertExpected)
	}

	if deps.MarkAlertEmitted("council_draft_quote", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal("expected duplicate alert emission to be suppressed")
	}
}

func TestMarkAlertEmittedAllowsDistinctAlerts(t *testing.T) {
	deps := &ToolDependencies{}

	if !deps.MarkAlertEmitted("council_draft_quote", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal(firstAlertExpected)
	}

	if !deps.MarkAlertEmitted("council_stage_update", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal("expected distinct alert category to be recorded")
	}

	if !deps.MarkAlertEmitted("council_draft_quote", "council_low_confidence_quote", "Council blokkeert conceptofferte: analysekans is te laag.") {
		t.Fatal("expected distinct reason code to be recorded")
	}
}

func TestResetToolCallTrackingClearsAlertDedupeState(t *testing.T) {
	deps := &ToolDependencies{}

	if !deps.MarkAlertEmitted("council_draft_quote", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal(firstAlertExpected)
	}

	deps.ResetToolCallTracking()

	if !deps.MarkAlertEmitted("council_draft_quote", "council_intake_not_ready_for_quote", blockedDraftQuoteSummary) {
		t.Fatal("expected alert emission to be allowed after resetting run state")
	}
}

func TestForceDraftQuoteTracksManualGovernanceBypass(t *testing.T) {
	deps := &ToolDependencies{}
	if deps.ShouldForceDraftQuote() {
		t.Fatal("expected manual governance bypass to be disabled by default")
	}

	deps.SetForceDraftQuote(true)

	if !deps.ShouldForceDraftQuote() {
		t.Fatal("expected manual governance bypass to be enabled")
	}
}
