package management

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

func TestBuildTimelineItems_HidesDebugEvents(t *testing.T) {
	serviceID := uuid.New()
	now := time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC)

	items := buildTimelineItems([]repository.TimelineEvent{
		{
			ID:         uuid.New(),
			ServiceID:  &serviceID,
			ActorType:  repository.ActorTypeAI,
			ActorName:  repository.ActorNamePhotoAnalysis,
			EventType:  repository.EventTypePhotoAnalysisCompleted,
			Title:      repository.EventTitlePhotoAnalysisCompleted,
			Visibility: repository.TimelineVisibilityDebug,
			CreatedAt:  now,
		},
		{
			ID:         uuid.New(),
			ServiceID:  &serviceID,
			ActorType:  repository.ActorTypeSystem,
			ActorName:  repository.ActorNameOrchestrator,
			EventType:  repository.EventTypeAlert,
			Title:      repository.EventTitlePhotoAnalysisFailed,
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now.Add(-time.Minute),
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 visible timeline item, got %d", len(items))
	}
	if items[0].Title != repository.EventTitlePhotoAnalysisFailed {
		t.Fatalf("expected non-debug alert to remain visible, got %q", items[0].Title)
	}
}

func TestBuildTimelineItems_SuppressesStandaloneAnalysisWhenStageCarriesContext(t *testing.T) {
	serviceID := uuid.New()
	now := time.Date(2026, time.March, 8, 13, 0, 0, 0, time.UTC)

	items := buildTimelineItems([]repository.TimelineEvent{
		{
			ID:        uuid.New(),
			ServiceID: &serviceID,
			ActorType: repository.ActorTypeAI,
			ActorName: repository.ActorNameGatekeeper,
			EventType: repository.EventTypeStageChange,
			Title:     repository.EventTitleStageUpdated,
			Metadata: map[string]any{
				"oldStage": "Triage",
				"newStage": "Ready_For_Estimator",
				"analysis": map[string]any{"recommendedAction": "schedule_survey"},
			},
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now,
		},
		{
			ID:         uuid.New(),
			ServiceID:  &serviceID,
			ActorType:  repository.ActorTypeAI,
			ActorName:  repository.ActorNameGatekeeper,
			EventType:  repository.EventTypeAI,
			Title:      repository.EventTitleGatekeeperAnalysis,
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now.Add(-time.Minute),
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected only the stage change to remain, got %d items", len(items))
	}
	if items[0].Type != "stage" {
		t.Fatalf("expected stage item, got %q", items[0].Type)
	}
}

func TestBuildTimelineItems_SuppressesEstimatorArtifactsWhenStageCarriesContext(t *testing.T) {
	serviceID := uuid.New()
	now := time.Date(2026, time.March, 8, 14, 0, 0, 0, time.UTC)

	items := buildTimelineItems([]repository.TimelineEvent{
		{
			ID:        uuid.New(),
			ServiceID: &serviceID,
			ActorType: repository.ActorTypeAI,
			ActorName: repository.ActorNameEstimator,
			EventType: repository.EventTypeStageChange,
			Title:     repository.EventTitleStageUpdated,
			Metadata: map[string]any{
				"oldStage":   "Ready_For_Estimator",
				"newStage":   "Proposal",
				"estimation": map[string]any{"scope": "Replace roof window", "priceRange": "€1.500-€2.000"},
				"draftQuote": map[string]any{"quoteNumber": "Q-2026-001", "itemCount": 3},
			},
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now,
		},
		{
			ID:         uuid.New(),
			ServiceID:  &serviceID,
			ActorType:  repository.ActorTypeAI,
			ActorName:  repository.ActorNameEstimator,
			EventType:  repository.EventTypeAnalysis,
			Title:      repository.EventTitleEstimationSaved,
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now.Add(-time.Minute),
		},
		{
			ID:         uuid.New(),
			ServiceID:  &serviceID,
			ActorType:  repository.ActorTypeSystem,
			ActorName:  repository.ActorNameEstimator,
			EventType:  "quote_drafted",
			Title:      "Draft quote Q-2026-001 created",
			Visibility: repository.TimelineVisibilityPublic,
			CreatedAt:  now.Add(-2 * time.Minute),
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected only the estimator stage change to remain, got %d items", len(items))
	}
	if items[0].Metadata["draftQuote"] == nil {
		t.Fatalf("expected stage change to retain draft quote metadata")
	}
}