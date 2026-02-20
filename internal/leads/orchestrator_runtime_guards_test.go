package leads

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
)

func newGuardOnlyOrchestrator() *Orchestrator {
	return &Orchestrator{
		activeRuns:             make(map[string]bool),
		activeReconciliations:  make(map[uuid.UUID]bool),
		recentStageEvents:      make(map[string]time.Time),
		pendingGatekeeperPhoto: make(map[uuid.UUID]events.PhotoAnalysisCompleted),
		orgSettingsCache:       make(map[uuid.UUID]cachedOrgAISettings),
	}
}

func TestMarkReconciliationRunning(t *testing.T) {
	o := newGuardOnlyOrchestrator()
	serviceID := uuid.New()

	if !o.markReconciliationRunning(serviceID) {
		t.Fatalf("expected first reconciliation lock acquisition to succeed")
	}
	if o.markReconciliationRunning(serviceID) {
		t.Fatalf("expected second reconciliation lock acquisition to fail")
	}

	o.markReconciliationComplete(serviceID)

	if !o.markReconciliationRunning(serviceID) {
		t.Fatalf("expected lock acquisition to succeed after completion")
	}
}

func TestShouldSkipDuplicateStageEvent(t *testing.T) {
	o := newGuardOnlyOrchestrator()
	serviceID := uuid.New()
	evt := events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        uuid.New(),
		LeadServiceID: serviceID,
		TenantID:      uuid.New(),
		OldStage:      "Triage",
		NewStage:      "Estimation",
	}

	if o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected first stage event to be accepted")
	}
	if !o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected immediate duplicate stage event to be skipped")
	}

	key := evt.LeadServiceID.String() + ":" + evt.OldStage + "->" + evt.NewStage
	o.recentStageEvents[key] = time.Now().Add(-stageEventDedupWindow - time.Second)

	if o.shouldSkipDuplicateStageEvent(evt) {
		t.Fatalf("expected stage event to be accepted after dedupe window elapsed")
	}
}
