package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
)

type gatekeeperAutoDisqualifyRepoStub struct {
	*repository.Repository
	analysis       repository.AIAnalysis
	timelineEvents []repository.CreateTimelineEventParams
	atomicUpdates  []struct {
		status string
		stage  string
	}
	stageUpdates  []string
	statusUpdates []string
}

func (s *gatekeeperAutoDisqualifyRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (repository.AIAnalysis, error) {
	return s.analysis, nil
}

func (s *gatekeeperAutoDisqualifyRepoStub) UpdateServiceStatusAndPipelineStage(_ context.Context, _ uuid.UUID, _ uuid.UUID, status string, stage string) (repository.LeadService, error) {
	s.atomicUpdates = append(s.atomicUpdates, struct {
		status string
		stage  string
	}{status: status, stage: stage})
	return repository.LeadService{Status: status, PipelineStage: stage}, nil
}

func (s *gatekeeperAutoDisqualifyRepoStub) UpdatePipelineStage(_ context.Context, _ uuid.UUID, _ uuid.UUID, stage string) (repository.LeadService, error) {
	s.stageUpdates = append(s.stageUpdates, stage)
	return repository.LeadService{PipelineStage: stage}, nil
}

func (s *gatekeeperAutoDisqualifyRepoStub) UpdateServiceStatus(_ context.Context, _ uuid.UUID, _ uuid.UUID, status string) (repository.LeadService, error) {
	s.statusUpdates = append(s.statusUpdates, status)
	return repository.LeadService{Status: status}, nil
}

func (s *gatekeeperAutoDisqualifyRepoStub) CreateTimelineEvent(_ context.Context, params repository.CreateTimelineEventParams) (repository.TimelineEvent, error) {
	s.timelineEvents = append(s.timelineEvents, params)
	return repository.TimelineEvent{}, nil
}

func TestGatekeeperMaybeAutoDisqualifyJunkUsesAtomicStateUpdate(t *testing.T) {
	tenantID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	settings := ports.DefaultOrganizationAISettings()
	settings.AIAutoDisqualifyJunk = true
	repo := &gatekeeperAutoDisqualifyRepoStub{
		analysis: repository.AIAnalysis{
			ID:                uuid.New(),
			LeadID:            leadID,
			OrganizationID:    tenantID,
			LeadServiceID:     serviceID,
			LeadQuality:       "Junk",
			RecommendedAction: "Disqualify",
		},
	}
	deps := (&ToolDependencies{Repo: repo}).NewRequestDeps()
	deps.SetTenantID(tenantID)
	deps.SetOrganizationAISettingsReader(func(context.Context, uuid.UUID) (ports.OrganizationAISettings, error) {
		return settings, nil
	})
	ctx := WithDependencies(context.Background(), deps)
	g := &Gatekeeper{repo: repo}
	service := repository.LeadService{
		ID:             serviceID,
		LeadID:         leadID,
		OrganizationID: tenantID,
		Status:         domain.LeadStatusPending,
		PipelineStage:  domain.PipelineStageNurturing,
	}

	g.maybeAutoDisqualifyJunk(ctx, leadID, serviceID, tenantID, service)

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
	if len(repo.timelineEvents) != 1 {
		t.Fatalf("expected one timeline event, got %d", len(repo.timelineEvents))
	}
}
