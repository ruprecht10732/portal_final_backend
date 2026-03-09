package leads

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/logger"
)

type orchestratorRepoStub struct {
	*repository.Repository
	service        repository.LeadService
	timelineEvents []repository.CreateTimelineEventParams
}

func (s *orchestratorRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *orchestratorRepoStub) CreateTimelineEvent(_ context.Context, params repository.CreateTimelineEventParams) (repository.TimelineEvent, error) {
	s.timelineEvents = append(s.timelineEvents, params)
	return repository.TimelineEvent{}, nil
}

type noopBus struct{}

func (noopBus) Publish(context.Context, events.Event) {
	// This test asserts repository side effects only, so published events are ignored.
}

func (noopBus) PublishSync(context.Context, events.Event) error { return nil }

func (noopBus) Subscribe(string, events.Handler) {
	// The orchestrator test does not rely on event subscriptions.
}

func (noopBus) Shutdown(context.Context) error {
	// The no-op bus does not spawn workers or hold resources.
	return nil
}

func TestHandleManualInterventionStageUsesLoopDetectionAlertMetadata(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	fingerprint := "duidelijke foto"
	repo := &orchestratorRepoStub{
		service: repository.LeadService{
			ID:                                 serviceID,
			LeadID:                             leadID,
			OrganizationID:                     tenantID,
			GatekeeperNurturingLoopCount:       3,
			GatekeeperNurturingLoopFingerprint: &fingerprint,
		},
	}
	o := &Orchestrator{
		repo:     repo,
		eventBus: noopBus{},
		sse:      sse.New(),
		log:      logger.New("development"),
	}

	o.handleManualInterventionStage(events.PipelineStageChanged{
		BaseEvent:     events.NewBaseEvent(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		TenantID:      tenantID,
		OldStage:      "Nurturing",
		NewStage:      "Manual_Intervention",
		Reason:        "Systeem: AI zat in een lus. Menselijke controle vereist.",
		ReasonCode:    "nurturing_loop_threshold",
		Trigger:       "ai_loop_detected",
		RunID:         "svc-run-123",
	})

	if len(repo.timelineEvents) != 1 {
		t.Fatalf("expected one manual intervention alert, got %d", len(repo.timelineEvents))
	}
	event := repo.timelineEvents[0]
	if event.Title != repository.EventTitleAILoopDetected {
		t.Fatalf("expected %q title, got %q", repository.EventTitleAILoopDetected, event.Title)
	}
	if event.ActorName != repository.ActorNameLoopDetector {
		t.Fatalf("expected actor %q, got %q", repository.ActorNameLoopDetector, event.ActorName)
	}
	if event.Summary == nil || *event.Summary != "Systeem: AI zat in een lus. Menselijke controle vereist." {
		t.Fatalf("unexpected summary: %#v", event.Summary)
	}
	if got := event.Metadata["attemptCount"]; got != float64(3) {
		t.Fatalf("expected attemptCount metadata, got %#v", got)
	}
	if got := event.Metadata["blockerFingerprint"]; got != fingerprint {
		t.Fatalf("expected blockerFingerprint metadata, got %#v", got)
	}
	if got := event.Metadata["trigger"]; got != "ai_loop_detected" {
		t.Fatalf("expected loop trigger metadata, got %#v", got)
	}
	if got := event.Metadata["reasonCode"]; got != "nurturing_loop_threshold" {
		t.Fatalf("expected reasonCode metadata, got %#v", got)
	}
	if got := event.Metadata["runId"]; got != "svc-run-123" {
		t.Fatalf("expected runId metadata, got %#v", got)
	}
}
