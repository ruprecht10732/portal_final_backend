package leads

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"
)

type atomicStateUpdate struct {
	status string
	stage  string
}

type orchestratorStateRepoStub struct {
	*repository.Repository
	service        repository.LeadService
	analysis       repository.AIAnalysis
	timelineEvents []repository.CreateTimelineEventParams
	atomicUpdates  []atomicStateUpdate
	stageUpdates   []string
	statusUpdates  []string
}

func (s *orchestratorStateRepoStub) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	return s.service, nil
}

func (s *orchestratorStateRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.AIAnalysis, error) {
	return s.analysis, nil
}

func (s *orchestratorStateRepoStub) CreateTimelineEvent(_ context.Context, params repository.CreateTimelineEventParams) (repository.TimelineEvent, error) {
	s.timelineEvents = append(s.timelineEvents, params)
	return repository.TimelineEvent{}, nil
}

func (s *orchestratorStateRepoStub) UpdateServiceStatusAndPipelineStage(_ context.Context, _ uuid.UUID, _ uuid.UUID, status string, stage string) (repository.LeadService, error) {
	s.atomicUpdates = append(s.atomicUpdates, atomicStateUpdate{status: status, stage: stage})
	s.service.Status = status
	s.service.PipelineStage = stage
	return s.service, nil
}

func (s *orchestratorStateRepoStub) UpdatePipelineStage(_ context.Context, _ uuid.UUID, _ uuid.UUID, stage string) (repository.LeadService, error) {
	s.stageUpdates = append(s.stageUpdates, stage)
	s.service.PipelineStage = stage
	return s.service, nil
}

func (s *orchestratorStateRepoStub) UpdateServiceStatus(_ context.Context, _ uuid.UUID, _ uuid.UUID, status string) (repository.LeadService, error) {
	s.statusUpdates = append(s.statusUpdates, status)
	s.service.Status = status
	return s.service, nil
}

type orchestratorStateBusStub struct {
	published []events.Event
}

func (b *orchestratorStateBusStub) Publish(_ context.Context, event events.Event) {
	b.published = append(b.published, event)
}

func (b *orchestratorStateBusStub) PublishSync(_ context.Context, event events.Event) error {
	b.published = append(b.published, event)
	return nil
}

func (b *orchestratorStateBusStub) Subscribe(string, events.Handler) {
	// Tests publish directly and do not rely on asynchronous subscriptions.
}

func (b *orchestratorStateBusStub) Shutdown(context.Context) error { return nil }

func TestOnQuoteRejectedMarksServiceDisqualifiedAndLostAtomically(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	quoteID := uuid.New()
	bus := &orchestratorStateBusStub{}
	repo := &orchestratorStateRepoStub{
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			Status:         domain.LeadStatusPending,
			PipelineStage:  domain.PipelineStageProposal,
		},
	}
	o := &Orchestrator{repo: repo, eventBus: bus, log: logger.New("development")}

	o.OnQuoteRejected(context.Background(), events.QuoteRejected{
		BaseEvent:      events.NewBaseEvent(),
		LeadID:         leadID,
		LeadServiceID:  &serviceID,
		OrganizationID: tenantID,
		QuoteID:        quoteID,
		Reason:         "te duur",
	})

	if len(repo.atomicUpdates) != 1 {
		t.Fatalf("expected one atomic state update, got %d", len(repo.atomicUpdates))
	}
	if update := repo.atomicUpdates[0]; update.status != domain.LeadStatusDisqualified || update.stage != domain.PipelineStageLost {
		t.Fatalf("expected Disqualified/Lost atomic update, got %+v", update)
	}
	if len(repo.stageUpdates) != 0 {
		t.Fatalf("expected no stage-only updates, got %v", repo.stageUpdates)
	}
	if len(repo.statusUpdates) != 0 {
		t.Fatalf("expected no status-only updates, got %v", repo.statusUpdates)
	}
	if repo.service.Status != domain.LeadStatusDisqualified || repo.service.PipelineStage != domain.PipelineStageLost {
		t.Fatalf("expected service state to be Disqualified/Lost, got %s/%s", repo.service.Status, repo.service.PipelineStage)
	}
	if len(bus.published) != 1 {
		t.Fatalf("expected one published stage event, got %d", len(bus.published))
	}
	evt, ok := bus.published[0].(events.PipelineStageChanged)
	if !ok {
		t.Fatalf("expected PipelineStageChanged event, got %T", bus.published[0])
	}
	if evt.OldStage != domain.PipelineStageProposal || evt.NewStage != domain.PipelineStageLost {
		t.Fatalf("unexpected stage event transition: %+v", evt)
	}
}

func TestMaybeAutoDisqualifyJunkUsesAtomicStateUpdate(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	settings := ports.DefaultOrganizationAISettings()
	settings.AIAutoDisqualifyJunk = true
	repo := &orchestratorStateRepoStub{
		service: repository.LeadService{
			ID:             serviceID,
			LeadID:         leadID,
			OrganizationID: tenantID,
			Status:         domain.LeadStatusPending,
			PipelineStage:  domain.PipelineStageNurturing,
		},
		analysis: repository.AIAnalysis{
			ID:                uuid.New(),
			LeadID:            leadID,
			OrganizationID:    tenantID,
			LeadServiceID:     serviceID,
			LeadQuality:       "Junk",
			RecommendedAction: "Disqualify",
		},
	}
	o := &Orchestrator{
		repo:              repo,
		eventBus:          &orchestratorStateBusStub{},
		log:               logger.New("development"),
		orgSettingsReader: func(context.Context, uuid.UUID) (ports.OrganizationAISettings, error) { return settings, nil },
		orgSettingsCache:  make(map[uuid.UUID]cachedOrgAISettings),
	}

	o.maybeAutoDisqualifyJunk(context.Background(), leadID, serviceID, tenantID)

	if len(repo.atomicUpdates) != 1 {
		t.Fatalf("expected one atomic state update, got %d", len(repo.atomicUpdates))
	}
	if update := repo.atomicUpdates[0]; update.status != domain.LeadStatusDisqualified || update.stage != domain.PipelineStageLost {
		t.Fatalf("expected Disqualified/Lost atomic update, got %+v", update)
	}
	if len(repo.stageUpdates) != 0 {
		t.Fatalf("expected no stage-only updates, got %v", repo.stageUpdates)
	}
	if len(repo.statusUpdates) != 0 {
		t.Fatalf("expected no status-only updates, got %v", repo.statusUpdates)
	}
}
