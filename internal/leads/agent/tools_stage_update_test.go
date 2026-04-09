package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
)

const (
	expectedNoErrorMessage = "expected no error, got %v"
	expectedSuccessMessage = "expected success, got %+v"
)

type stageUpdateRepoStub struct {
	*repository.Repository
	service         repository.LeadService
	analysis        repository.AIAnalysis
	hasAnalysis     bool
	timelineEvents  []repository.CreateTimelineEventParams
	updatedStages   []string
	lastLoopCount   int
	lastFingerprint string
	resetCalls      int
}

func (s *stageUpdateRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *stageUpdateRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.AIAnalysis, error) {
	if !s.hasAnalysis {
		return repository.AIAnalysis{}, repository.ErrNotFound
	}
	return s.analysis, nil
}

func (s *stageUpdateRepoStub) SetGatekeeperNurturingLoopState(_ context.Context, _ uuid.UUID, _ uuid.UUID, count int, fingerprint string) error {
	s.lastLoopCount = count
	s.lastFingerprint = fingerprint
	s.service.GatekeeperNurturingLoopCount = count
	s.service.GatekeeperNurturingLoopFingerprint = &fingerprint
	return nil
}

func (s *stageUpdateRepoStub) ResetGatekeeperNurturingLoopState(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	s.resetCalls++
	s.service.GatekeeperNurturingLoopCount = 0
	s.service.GatekeeperNurturingLoopFingerprint = nil
	return nil
}

func (s *stageUpdateRepoStub) UpdatePipelineStage(_ context.Context, _ uuid.UUID, _ uuid.UUID, stage string) (repository.LeadService, error) {
	s.updatedStages = append(s.updatedStages, stage)
	s.service.PipelineStage = stage
	return s.service, nil
}

func (s *stageUpdateRepoStub) CreateTimelineEvent(_ context.Context, params repository.CreateTimelineEventParams) (repository.TimelineEvent, error) {
	s.timelineEvents = append(s.timelineEvents, params)
	return repository.TimelineEvent{}, nil
}

type stageUpdateBusStub struct {
	published []events.Event
}

func (b *stageUpdateBusStub) Publish(_ context.Context, event events.Event) {
	b.published = append(b.published, event)
}

func (b *stageUpdateBusStub) PublishSync(_ context.Context, event events.Event) error {
	b.published = append(b.published, event)
	return nil
}

func (b *stageUpdateBusStub) Subscribe(string, events.Handler) {
	// Tests publish directly and do not need asynchronous subscriptions.
}

func (b *stageUpdateBusStub) Shutdown(context.Context) error {
	// The stub does not allocate background resources, so shutdown is a no-op.
	return nil
}

func newStageUpdateDeps(repo repository.LeadsRepository, bus events.Bus, tenantID, leadID, serviceID uuid.UUID) *ToolDependencies {
	deps := (&ToolDependencies{Repo: repo, EventBus: bus}).NewRequestDeps()
	deps.SetTenantID(tenantID)
	deps.SetLeadContext(leadID, serviceID)
	deps.SetActor(repository.ActorTypeAI, repository.ActorNameGatekeeper)
	deps.MarkSaveAnalysisCalled()
	return deps
}

func TestApplyPipelineStageUpdateCountsRepeatedGatekeeperNurturingWithoutStageChange(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	fingerprint := "duidelijke foto"
	repo := &stageUpdateRepoStub{
		service: repository.LeadService{
			ID:                                 serviceID,
			LeadID:                             leadID,
			OrganizationID:                     tenantID,
			Status:                             domain.LeadStatusNew,
			PipelineStage:                      domain.PipelineStageNurturing,
			GatekeeperNurturingLoopCount:       1,
			GatekeeperNurturingLoopFingerprint: &fingerprint,
		},
		hasAnalysis: true,
		analysis: repository.AIAnalysis{
			RecommendedAction:  "RequestInfo",
			MissingInformation: []string{"Duidelijke foto"},
		},
	}
	deps := newStageUpdateDeps(repo, nil, tenantID, leadID, serviceID)

	out, err := applyPipelineStageUpdate(context.Background(), deps, UpdatePipelineStageInput{
		Stage:  domain.PipelineStageNurturing,
		Reason: "Vraag een duidelijke foto op.",
	})
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	if out.Success {
		t.Fatalf("expected Success=false for same-stage transition, got %+v", out)
	}
	if repo.service.GatekeeperNurturingLoopCount != 2 {
		t.Fatalf("expected loop count to increment to 2, got %d", repo.service.GatekeeperNurturingLoopCount)
	}
	if len(repo.updatedStages) != 0 {
		t.Fatalf("expected no stage write for repeated nurturing, got %v", repo.updatedStages)
	}
	if len(repo.timelineEvents) != 0 {
		t.Fatalf("expected no timeline event for repeated nurturing without escalation, got %d", len(repo.timelineEvents))
	}
	if !deps.WasStageUpdateCalled() {
		t.Fatal("expected repeated nurturing attempt to count as a stage-update tool call")
	}
}

func TestApplyPipelineStageUpdateTripsLoopBreakerAtThreshold(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	fingerprint := "duidelijke foto"
	bus := &stageUpdateBusStub{}
	repo := &stageUpdateRepoStub{
		service: repository.LeadService{
			ID:                                 serviceID,
			LeadID:                             leadID,
			OrganizationID:                     tenantID,
			Status:                             domain.LeadStatusNew,
			PipelineStage:                      domain.PipelineStageNurturing,
			GatekeeperNurturingLoopCount:       2,
			GatekeeperNurturingLoopFingerprint: &fingerprint,
		},
		hasAnalysis: true,
		analysis: repository.AIAnalysis{
			RecommendedAction:  "RequestInfo",
			MissingInformation: []string{"Duidelijke foto"},
		},
	}
	deps := newStageUpdateDeps(repo, bus, tenantID, leadID, serviceID)

	out, err := applyPipelineStageUpdate(context.Background(), deps, UpdatePipelineStageInput{
		Stage:  domain.PipelineStageNurturing,
		Reason: "Vraag een duidelijke foto op.",
	})
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	if !out.Success {
		t.Fatalf(expectedSuccessMessage, out)
	}
	if len(repo.updatedStages) != 1 || repo.updatedStages[0] != domain.PipelineStageManualIntervention {
		t.Fatalf("expected manual intervention stage write, got %v", repo.updatedStages)
	}
	if repo.service.GatekeeperNurturingLoopCount != 3 {
		t.Fatalf("expected loop count to persist at threshold, got %d", repo.service.GatekeeperNurturingLoopCount)
	}
	if len(repo.timelineEvents) != 1 {
		t.Fatalf("expected one stage-change timeline event, got %d", len(repo.timelineEvents))
	}
	loopMeta, ok := repo.timelineEvents[0].Metadata["loopDetected"].(map[string]any)
	if !ok {
		t.Fatalf("expected loopDetected metadata on stage change, got %#v", repo.timelineEvents[0].Metadata)
	}
	if loopMeta["trigger"] != gatekeeperLoopDetectedTrigger {
		t.Fatalf("expected trigger %q, got %#v", gatekeeperLoopDetectedTrigger, loopMeta["trigger"])
	}
	if len(bus.published) != 1 {
		t.Fatalf("expected one published stage event, got %d", len(bus.published))
	}
	evt, ok := bus.published[0].(events.PipelineStageChanged)
	if !ok {
		t.Fatalf("expected PipelineStageChanged event, got %T", bus.published[0])
	}
	if evt.NewStage != domain.PipelineStageManualIntervention {
		t.Fatalf("expected Manual_Intervention event, got %s", evt.NewStage)
	}
	if evt.Trigger != gatekeeperLoopDetectedTrigger {
		t.Fatalf("expected trigger %q, got %q", gatekeeperLoopDetectedTrigger, evt.Trigger)
	}
	if evt.Reason != gatekeeperLoopDetectedSummary {
		t.Fatalf("expected loop-detected reason, got %q", evt.Reason)
	}
}

func TestApplyPipelineStageUpdateResetsLoopCountWhenMissingInformationChanges(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	oldFingerprint := "oude foto"
	repo := &stageUpdateRepoStub{
		service: repository.LeadService{
			ID:                                 serviceID,
			LeadID:                             leadID,
			OrganizationID:                     tenantID,
			Status:                             domain.LeadStatusNew,
			PipelineStage:                      domain.PipelineStageNurturing,
			GatekeeperNurturingLoopCount:       2,
			GatekeeperNurturingLoopFingerprint: &oldFingerprint,
		},
		hasAnalysis: true,
		analysis: repository.AIAnalysis{
			RecommendedAction:  "RequestInfo",
			MissingInformation: []string{"Meterbreedte ontbreekt"},
		},
	}
	deps := newStageUpdateDeps(repo, nil, tenantID, leadID, serviceID)

	out, err := applyPipelineStageUpdate(context.Background(), deps, UpdatePipelineStageInput{
		Stage:  domain.PipelineStageNurturing,
		Reason: "Vraag de meterbreedte op.",
	})
	if err != nil {
		t.Fatalf(expectedNoErrorMessage, err)
	}
	if out.Success {
		t.Fatalf("expected Success=false for same-stage transition, got %+v", out)
	}
	if repo.service.GatekeeperNurturingLoopCount != 1 {
		t.Fatalf("expected loop count reset to 1 for a new blocker, got %d", repo.service.GatekeeperNurturingLoopCount)
	}
	if repo.service.GatekeeperNurturingLoopFingerprint == nil || *repo.service.GatekeeperNurturingLoopFingerprint == oldFingerprint {
		t.Fatalf("expected fingerprint to change, got %#v", repo.service.GatekeeperNurturingLoopFingerprint)
	}
}
