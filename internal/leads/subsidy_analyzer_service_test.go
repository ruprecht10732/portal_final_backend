package leads

import (
	"context"
	"testing"
	"time"

	"portal_final_backend/internal/leads/repository"
	quoterepo "portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/scheduler"

	"github.com/google/uuid"
)

type fakeSubsidyAnalyzerQuoteStore struct {
	quote *quoterepo.Quote
	items []quoterepo.QuoteItem
	err   error
}

func (f *fakeSubsidyAnalyzerQuoteStore) GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*quoterepo.Quote, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.quote, nil
}

func (f *fakeSubsidyAnalyzerQuoteStore) GetItemsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]quoterepo.QuoteItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

type fakeSubsidyAnalyzerJobStore struct {
	jobs map[uuid.UUID]SubsidyAnalyzerJob
}

func (f *fakeSubsidyAnalyzerJobStore) CreateSubsidyAnalyzerJob(ctx context.Context, params repository.CreateSubsidyAnalyzerJobParams) (repository.SubsidyAnalyzerJob, error) {
	job := repository.SubsidyAnalyzerJob{
		ID:              params.ID,
		OrganizationID:  params.OrganizationID,
		UserID:          params.UserID,
		QuoteID:         params.QuoteID,
		Status:          params.Status,
		Step:            params.Step,
		ProgressPercent: params.ProgressPercent,
		CreatedAt:       params.CreatedAt,
		UpdatedAt:       params.UpdatedAt,
	}
	if f.jobs == nil {
		f.jobs = make(map[uuid.UUID]SubsidyAnalyzerJob)
	}
	f.jobs[job.ID] = *fromRepositorySubsidyAnalyzerJob(job)
	return job, nil
}

func (f *fakeSubsidyAnalyzerJobStore) GetSubsidyAnalyzerJob(ctx context.Context, jobID uuid.UUID, organizationID uuid.UUID) (repository.SubsidyAnalyzerJob, error) {
	job := f.jobs[jobID]
	return repository.SubsidyAnalyzerJob{
		ID:              job.ID,
		OrganizationID:  job.OrganizationID,
		UserID:          job.UserID,
		QuoteID:         job.QuoteID,
		Status:          job.Status,
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		Result:          job.Result,
		Error:           job.Error,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		FinishedAt:      job.FinishedAt,
		UpdatedAt:       job.UpdatedAt,
	}, nil
}

func (f *fakeSubsidyAnalyzerJobStore) UpdateSubsidyAnalyzerJob(ctx context.Context, params repository.UpdateSubsidyAnalyzerJobParams) (repository.SubsidyAnalyzerJob, error) {
	job := f.jobs[params.ID]
	job.Status = params.Status
	job.Step = params.Step
	job.ProgressPercent = params.ProgressPercent
	job.Error = params.Error
	job.Result = params.Result
	job.UpdatedAt = params.UpdatedAt
	if job.StartedAt == nil && params.StartedAt != nil {
		job.StartedAt = params.StartedAt
	}
	job.FinishedAt = params.FinishedAt
	f.jobs[params.ID] = job
	return repository.SubsidyAnalyzerJob{
		ID:              job.ID,
		OrganizationID:  job.OrganizationID,
		UserID:          job.UserID,
		QuoteID:         job.QuoteID,
		Status:          job.Status,
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		Result:          job.Result,
		Error:           job.Error,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		FinishedAt:      job.FinishedAt,
		UpdatedAt:       job.UpdatedAt,
	}, nil
}

type fakeSubsidyAnalyzerRunner struct {
	result map[string]interface{}
	err    error
}

func (f *fakeSubsidyAnalyzerRunner) Run(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID, quoteContext string) (map[string]interface{}, error) {
	return f.result, f.err
}

type fakeSubsidyAnalyzerScheduler struct {
	payloads []scheduler.SubsidyAnalyzerJobPayload
	err      error
}

func (f *fakeSubsidyAnalyzerScheduler) EnqueueSubsidyAnalyzerJob(ctx context.Context, payload scheduler.SubsidyAnalyzerJobPayload) error {
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, payload)
	return nil
}

func TestStartSubsidyAnalysisJobPersistsPendingJobAndEnqueuesTask(t *testing.T) {
	quoteID := uuid.New()
	organizationID := uuid.New()
	userID := uuid.New()
	quoteStore := &fakeSubsidyAnalyzerQuoteStore{
		quote: &quoterepo.Quote{ID: quoteID, OrganizationID: organizationID},
		items: []quoterepo.QuoteItem{{ID: uuid.New(), QuoteID: quoteID, OrganizationID: organizationID, Description: "Dakisolatie", Quantity: "1"}},
	}
	jobStore := &fakeSubsidyAnalyzerJobStore{jobs: make(map[uuid.UUID]SubsidyAnalyzerJob)}
	schedulerClient := &fakeSubsidyAnalyzerScheduler{}
	svc := &SubsidyAnalyzerService{
		quoteRepo:       quoteStore,
		jobStore:        jobStore,
		schedulerClient: schedulerClient,
	}

	jobID, err := svc.StartSubsidyAnalysisJob(context.Background(), quoteID, userID, organizationID, organizationID)
	if err != nil {
		t.Fatalf("StartSubsidyAnalysisJob returned error: %v", err)
	}

	job := jobStore.jobs[jobID]
	if job.Status != "pending" {
		t.Fatalf("expected pending job status, got %q", job.Status)
	}
	if len(schedulerClient.payloads) != 1 {
		t.Fatalf("expected one enqueued task, got %d", len(schedulerClient.payloads))
	}
	if schedulerClient.payloads[0].JobID != jobID.String() {
		t.Fatalf("expected payload job ID %s, got %s", jobID, schedulerClient.payloads[0].JobID)
	}
}

func TestProcessSubsidyAnalysisJobRunsAnalyzerAndStoresCompletedResult(t *testing.T) {
	jobID := uuid.New()
	quoteID := uuid.New()
	organizationID := uuid.New()
	userID := uuid.New()
	now := time.Now()
	quoteStore := &fakeSubsidyAnalyzerQuoteStore{
		quote: &quoterepo.Quote{ID: quoteID, OrganizationID: organizationID},
		items: []quoterepo.QuoteItem{{ID: uuid.New(), QuoteID: quoteID, OrganizationID: organizationID, Title: "Dakisolatie", Description: "PIR platen", Quantity: "1"}},
	}
	jobStore := &fakeSubsidyAnalyzerJobStore{jobs: map[uuid.UUID]SubsidyAnalyzerJob{
		jobID: {
			ID:             jobID,
			OrganizationID: organizationID,
			UserID:         userID,
			QuoteID:        quoteID,
			Status:         "pending",
			Step:           "Queued",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}}
	analyzer := &fakeSubsidyAnalyzerRunner{result: map[string]interface{}{
		"measure_type_ids": []string{"roof"},
		"measure_type_id":  "roof",
		"confidence":       "high",
	}}
	svc := &SubsidyAnalyzerService{
		quoteRepo: quoteStore,
		jobStore:  jobStore,
		analyzer:  analyzer,
	}

	if err := svc.ProcessSubsidyAnalysisJob(context.Background(), jobID, quoteID, organizationID); err != nil {
		t.Fatalf("ProcessSubsidyAnalysisJob returned error: %v", err)
	}

	job := jobStore.jobs[jobID]
	if job.Status != "completed" {
		t.Fatalf("expected completed job status, got %q", job.Status)
	}
	if job.ProgressPercent != 100 {
		t.Fatalf("expected progress 100, got %d", job.ProgressPercent)
	}
	if job.Result["measure_type_id"] != "roof" {
		t.Fatalf("expected persisted result, got %#v", job.Result)
	}
	if job.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if job.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set")
	}
}
