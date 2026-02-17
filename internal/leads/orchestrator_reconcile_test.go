package leads

import (
	"testing"
	"time"

	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
)

func TestDeriveDesiredServiceState_TerminalNoResurrectionWhenTriggerBeforeTerminalAt(t *testing.T) {
	now := time.Now()
	terminalAt := now

	current := repository.LeadService{
		Status:        domain.LeadStatusLost,
		PipelineStage: domain.PipelineStageLost,
		UpdatedAt:     now,
	}
	aggs := repository.ServiceStateAggregates{
		DraftQuotes: 1,
		TerminalAt:  &terminalAt,
	}

	_, ok := deriveDesiredServiceState(current, aggs, true, now.Add(-1*time.Minute))
	if ok {
		t.Fatalf("expected reconciliation to be skipped (no resurrection), got ok=true")
	}
}

func TestDeriveDesiredServiceState_TerminalResurrectsWhenTriggerAfterTerminalAt(t *testing.T) {
	now := time.Now()
	terminalAt := now.Add(-1 * time.Hour)

	current := repository.LeadService{
		Status:        domain.LeadStatusLost,
		PipelineStage: domain.PipelineStageLost,
		UpdatedAt:     terminalAt,
	}
	aggs := repository.ServiceStateAggregates{
		SentQuotes:     1,
		TerminalAt:     &terminalAt,
		AiAction:       nil,
		HasVisitReport: false,
	}

	desired, ok := deriveDesiredServiceState(current, aggs, true, now)
	if !ok {
		t.Fatalf("expected reconciliation to proceed, got ok=false")
	}
	if !desired.Resurrecting {
		t.Fatalf("expected Resurrecting=true")
	}
	if desired.Stage != domain.PipelineStageQuoteSent {
		t.Fatalf("expected stage=%q, got %q", domain.PipelineStageQuoteSent, desired.Stage)
	}
	if desired.Status != domain.LeadStatusQuoteSent {
		t.Fatalf("expected status=%q, got %q", domain.LeadStatusQuoteSent, desired.Status)
	}
	if desired.ReasonCode != "terminal_resurrection" {
		t.Fatalf("expected reasonCode=%q, got %q", "terminal_resurrection", desired.ReasonCode)
	}
}

func TestDeriveDesiredServiceState_StaleDraftDecaysToNurturing(t *testing.T) {
	now := time.Now()
	old := now.Add(-(staleDraftDuration + 2*time.Hour))

	current := repository.LeadService{
		Status:        domain.LeadStatusQuoteDraft,
		PipelineStage: domain.PipelineStageQuoteDraft,
		UpdatedAt:     now,
	}
	aggs := repository.ServiceStateAggregates{
		DraftQuotes:   1,
		LatestQuoteAt: &old,
	}

	desired, ok := deriveDesiredServiceState(current, aggs, false, now)
	if !ok {
		t.Fatalf("expected reconciliation to proceed, got ok=false")
	}
	if desired.Stage != domain.PipelineStageNurturing {
		t.Fatalf("expected stage=%q, got %q", domain.PipelineStageNurturing, desired.Stage)
	}
	if desired.Status != domain.LeadStatusAttemptedContact {
		t.Fatalf("expected status=%q, got %q", domain.LeadStatusAttemptedContact, desired.Status)
	}
	if desired.ReasonCode != "stale_draft_decay" {
		t.Fatalf("expected reasonCode=%q, got %q", "stale_draft_decay", desired.ReasonCode)
	}
}

func TestDeriveDesiredServiceState_ScheduledAppointmentSetsNurturingStage(t *testing.T) {
	now := time.Now()

	current := repository.LeadService{
		Status:        domain.LeadStatusNew,
		PipelineStage: domain.PipelineStageTriage,
		UpdatedAt:     now,
	}
	aggs := repository.ServiceStateAggregates{
		ScheduledAppointments: 1,
	}

	desired, ok := deriveDesiredServiceState(current, aggs, false, now)
	if !ok {
		t.Fatalf("expected reconciliation to proceed, got ok=false")
	}
	if desired.Stage != domain.PipelineStageNurturing {
		t.Fatalf("expected stage=%q, got %q", domain.PipelineStageNurturing, desired.Stage)
	}
	if desired.Status != domain.LeadStatusAppointmentScheduled {
		t.Fatalf("expected status=%q, got %q", domain.LeadStatusAppointmentScheduled, desired.Status)
	}
}

func TestShouldResurrect_FallsBackToServiceUpdatedAtWhenTerminalAtMissing(t *testing.T) {
	now := time.Now()
	terminalUpdatedAt := now.Add(-30 * time.Minute)

	current := repository.LeadService{
		Status:        domain.LeadStatusLost,
		PipelineStage: domain.PipelineStageLost,
		UpdatedAt:     terminalUpdatedAt,
	}
	aggs := repository.ServiceStateAggregates{SentQuotes: 1}

	if shouldResurrect(current, aggs, true, terminalUpdatedAt) {
		t.Fatalf("expected no resurrection when triggerAt == terminal updated_at")
	}
	if !shouldResurrect(current, aggs, true, terminalUpdatedAt.Add(1*time.Second)) {
		t.Fatalf("expected resurrection when triggerAt is after terminal updated_at")
	}
}
