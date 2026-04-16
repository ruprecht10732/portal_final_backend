package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	leadagent "portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	quoterepo "portal_final_backend/internal/quotes/repository"
	quotetransport "portal_final_backend/internal/quotes/transport"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/logger"
)

// SubsidyAnalyzerJob represents a subsidy analysis job.
type SubsidyAnalyzerJob struct {
	ID              uuid.UUID              `json:"id"`
	OrganizationID  uuid.UUID              `json:"organizationId"`
	UserID          uuid.UUID              `json:"userId"`
	QuoteID         uuid.UUID              `json:"quoteId"`
	Status          string                 `json:"status"`
	Step            string                 `json:"step"`
	ProgressPercent int                    `json:"progressPercent"`
	Result          map[string]interface{} `json:"result,omitempty"`
	Error           *string                `json:"error,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	StartedAt       *time.Time             `json:"startedAt,omitempty"`
	FinishedAt      *time.Time             `json:"finishedAt,omitempty"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

type subsidyAnalyzerQuoteStore interface {
	GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*quoterepo.Quote, error)
	GetItemsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]quoterepo.QuoteItem, error)
}

type subsidyAnalyzerJobStore interface {
	CreateSubsidyAnalyzerJob(ctx context.Context, params repository.CreateSubsidyAnalyzerJobParams) (repository.SubsidyAnalyzerJob, error)
	GetSubsidyAnalyzerJob(ctx context.Context, jobID uuid.UUID, organizationID uuid.UUID) (repository.SubsidyAnalyzerJob, error)
	UpdateSubsidyAnalyzerJob(ctx context.Context, params repository.UpdateSubsidyAnalyzerJobParams) (repository.SubsidyAnalyzerJob, error)
}

type subsidyAnalyzerRunner interface {
	Run(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID, quoteContext string) (map[string]interface{}, error)
}

type subsidyAnalyzerScheduler interface {
	EnqueueSubsidyAnalyzerJob(ctx context.Context, payload scheduler.SubsidyAnalyzerJobPayload) error
}

// SubsidyAnalyzerService handles subsidy analysis job orchestration.
type SubsidyAnalyzerService struct {
	repo            repository.LeadsRepository
	quoteRepo       subsidyAnalyzerQuoteStore
	jobStore        subsidyAnalyzerJobStore
	analyzer        subsidyAnalyzerRunner
	eventBus        events.Bus
	sseService      *sse.Service
	schedulerClient subsidyAnalyzerScheduler
	log             *logger.Logger
	modelConfig     openaicompat.Config
}

// SubsidyAnalyzerServiceConfig holds dependencies for the service.
type SubsidyAnalyzerServiceConfig struct {
	Repo            repository.LeadsRepository
	QuoteRepo       *quoterepo.Repository
	EventBus        events.Bus
	SSEService      *sse.Service
	SchedulerClient *scheduler.Client
	Log             *logger.Logger
	ModelConfig     openaicompat.Config
}

// NewSubsidyAnalyzerService creates a new subsidy analyzer service.
func NewSubsidyAnalyzerService(cfg SubsidyAnalyzerServiceConfig) *SubsidyAnalyzerService {
	var jobStore subsidyAnalyzerJobStore
	if store, ok := cfg.Repo.(subsidyAnalyzerJobStore); ok {
		jobStore = store
	}

	return &SubsidyAnalyzerService{
		repo:            cfg.Repo,
		quoteRepo:       cfg.QuoteRepo,
		jobStore:        jobStore,
		analyzer:        nil,
		eventBus:        cfg.EventBus,
		sseService:      cfg.SSEService,
		schedulerClient: cfg.SchedulerClient,
		log:             cfg.Log,
		modelConfig:     cfg.ModelConfig,
	}
}

// AnalyzeDraftSubsidy analyzes current unsaved line items synchronously and returns an AI suggestion.
func (s *SubsidyAnalyzerService) AnalyzeDraftSubsidy(ctx context.Context, organizationID uuid.UUID, items []quotetransport.QuoteItemRequest) (map[string]interface{}, error) {
	analyzer, err := s.ensureAnalyzer()
	if err != nil {
		return nil, err
	}

	quoteContext := buildDraftQuoteContext(items)
	if strings.TrimSpace(quoteContext) == "" {
		return map[string]interface{}{}, nil
	}

	return analyzer.Run(ctx, uuid.New(), organizationID, quoteContext)
}

func (s *SubsidyAnalyzerService) ensureAnalyzer() (subsidyAnalyzerRunner, error) {
	if s.analyzer != nil {
		return s.analyzer, nil
	}
	if s.modelConfig.APIKey == "" {
		return nil, fmt.Errorf("llm api key not configured for subsidy analyzer")
	}

	runner, err := leadagent.NewSubsidyAnalyzerAgent(leadagent.SubsidyAnalyzerConfig{
		ModelConfig: s.modelConfig,
		Repo:        s.repo,
	})
	if err != nil {
		return nil, err
	}

	s.analyzer = runner
	return s.analyzer, nil
}

func buildDraftQuoteContext(items []quotetransport.QuoteItemRequest) string {
	var builder strings.Builder
	builder.WriteString("Quote line items:\n")
	for index, item := range items {
		fmt.Fprintf(&builder, "%d. ", index+1)
		if item.Title != "" {
			builder.WriteString(item.Title)
			builder.WriteString(" - ")
		}
		builder.WriteString(item.Description)
		fmt.Fprintf(&builder, " | quantity: %s", item.Quantity)
		if item.IsOptional {
			builder.WriteString(" | optional")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildPersistedQuoteContext(quote *quoterepo.Quote, items []quoterepo.QuoteItem) string {
	var builder strings.Builder

	builder.WriteString("Quote line items:\n")
	for index, item := range items {
		fmt.Fprintf(&builder, "%d. ", index+1)
		label := strings.TrimSpace(item.Title)
		if label == "" {
			label = strings.TrimSpace(item.Description)
		}
		builder.WriteString(label)
		if item.Description != "" && item.Description != label {
			builder.WriteString(" - ")
			builder.WriteString(item.Description)
		}
		fmt.Fprintf(&builder, " | quantity: %s", item.Quantity)
		if item.IsOptional {
			builder.WriteString(" | optional")
		}
		builder.WriteString("\n")
	}

	if quote != nil && quote.Notes != nil && strings.TrimSpace(*quote.Notes) != "" {
		builder.WriteString("\nQuote notes:\n")
		builder.WriteString(strings.TrimSpace(*quote.Notes))
		builder.WriteString("\n")
	}

	return builder.String()
}

// SetQuoteRepo injects the quote repository after initialization.
func (s *SubsidyAnalyzerService) SetQuoteRepo(repo quoterepo.Repository) {
	s.quoteRepo = &repo
}

// SetSchedulerClient injects the scheduler client after initialization.
func (s *SubsidyAnalyzerService) SetSchedulerClient(client scheduler.Client) {
	s.schedulerClient = &client
}

// StartSubsidyAnalysisJob creates a new subsidy analysis job and enqueues the processing task.
func (s *SubsidyAnalyzerService) StartSubsidyAnalysisJob(ctx context.Context, quoteID uuid.UUID, userID uuid.UUID, tenantID uuid.UUID, organizationID uuid.UUID) (uuid.UUID, error) {
	if s.jobStore == nil {
		return uuid.Nil, fmt.Errorf("subsidy analyzer job store not configured")
	}
	if s.quoteRepo == nil {
		return uuid.Nil, fmt.Errorf("quote repository not configured for subsidy analyzer")
	}

	if _, err := s.quoteRepo.GetByID(ctx, quoteID, organizationID); err != nil {
		return uuid.Nil, err
	}

	items, err := s.quoteRepo.GetItemsByQuoteID(ctx, quoteID, organizationID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(items) == 0 {
		return uuid.Nil, apperr.Validation("quote has no line items to analyze")
	}

	jobID := uuid.New()
	now := time.Now()

	_, err = s.jobStore.CreateSubsidyAnalyzerJob(ctx, repository.CreateSubsidyAnalyzerJobParams{
		ID:              jobID,
		OrganizationID:  organizationID,
		UserID:          userID,
		QuoteID:         quoteID,
		Status:          "pending",
		Step:            "Queued",
		ProgressPercent: 0,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create subsidy analyzer job: %w", err)
	}

	// Enqueue async task
	if s.schedulerClient == nil {
		return uuid.Nil, fmt.Errorf("scheduler client not configured for subsidy analyzer")
	}

	taskPayload := scheduler.SubsidyAnalyzerJobPayload{
		JobID:          jobID.String(),
		TenantID:       tenantID.String(),
		UserID:         userID.String(),
		QuoteID:        quoteID.String(),
		OrganizationID: organizationID.String(),
	}

	err = s.schedulerClient.EnqueueSubsidyAnalyzerJob(ctx, taskPayload)
	if err != nil {
		s.updateJobStatus(ctx, jobID, "failed", "Queueing failed", 0, nil, ptr(err.Error()))
		return uuid.Nil, fmt.Errorf("failed to enqueue subsidy analyzer task: %w", err)
	}

	return jobID, nil
}

// GetSubsidyAnalysisJob fetches the current status of a job.
func (s *SubsidyAnalyzerService) GetSubsidyAnalysisJob(ctx context.Context, jobID uuid.UUID, organizationID uuid.UUID) (interface{}, error) {
	if s.jobStore == nil {
		return nil, fmt.Errorf("subsidy analyzer job store not configured")
	}

	job, err := s.jobStore.GetSubsidyAnalyzerJob(ctx, jobID, organizationID)
	if err != nil {
		return nil, err
	}

	return fromRepositorySubsidyAnalyzerJob(job), nil
}

// ProcessSubsidyAnalysisJob is the main orchestration logic for analyzing subsidies.
// This is called by the scheduler worker.
func (s *SubsidyAnalyzerService) ProcessSubsidyAnalysisJob(ctx context.Context, jobID uuid.UUID, quoteID uuid.UUID, organizationID uuid.UUID) error {
	if s.jobStore == nil {
		return fmt.Errorf("subsidy analyzer job store not configured")
	}
	if s.quoteRepo == nil {
		return fmt.Errorf("quote repository not configured for subsidy analyzer")
	}

	analyzer, err := s.ensureAnalyzer()
	if err != nil {
		s.updateJobStatus(ctx, jobID, "failed", "Analyzer setup failed", 0, nil, ptr(err.Error()))
		return err
	}

	if _, err := s.updateJobProgress(ctx, jobID, "running", "Loading quote context", 10, nil); err != nil {
		return err
	}

	// Step 1: Log job start (actual quote loading happens in the agent)
	if s.log != nil {
		s.log.Info("subsidy analysis job started", "jobId", jobID.String(), "quoteId", quoteID.String())
	}

	quote, err := s.quoteRepo.GetByID(ctx, quoteID, organizationID)
	if err != nil {
		s.updateJobStatus(ctx, jobID, "failed", "Loading quote failed", 10, nil, ptr(err.Error()))
		return err
	}

	items, err := s.quoteRepo.GetItemsByQuoteID(ctx, quoteID, organizationID)
	if err != nil {
		s.updateJobStatus(ctx, jobID, "failed", "Loading quote items failed", 20, nil, ptr(err.Error()))
		return err
	}
	if len(items) == 0 {
		err = apperr.Validation("quote has no line items to analyze")
		s.updateJobStatus(ctx, jobID, "failed", "No quote items found", 20, nil, ptr(err.Error()))
		return err
	}

	if _, err := s.updateJobProgress(ctx, jobID, "running", "Preparing quote context", 30, nil); err != nil {
		return err
	}

	quoteContext := buildPersistedQuoteContext(quote, items)
	if strings.TrimSpace(quoteContext) == "" {
		err = apperr.Validation("quote context is empty")
		s.updateJobStatus(ctx, jobID, "failed", "Quote context is empty", 30, nil, ptr(err.Error()))
		return err
	}

	if _, err := s.updateJobProgress(ctx, jobID, "running", "Analyzing with AI", 60, nil); err != nil {
		return err
	}

	result, err := analyzer.Run(ctx, quoteID, organizationID, quoteContext)
	if err != nil {
		s.updateJobStatus(ctx, jobID, "failed", "Analysis failed", 60, nil, ptr(err.Error()))
		return err
	}

	if _, err := s.updateJobProgress(ctx, jobID, "running", "Finalizing result", 90, nil); err != nil {
		return err
	}

	if _, err := s.updateJobStatus(ctx, jobID, "completed", "Completed", 100, result, nil); err != nil {
		return err
	}

	return nil
}

// updateJobProgress updates job status and emits SSE event.

func (s *SubsidyAnalyzerService) updateJobProgress(ctx context.Context, jobID uuid.UUID, status string, step string, progress int, errorMsg *string) (*SubsidyAnalyzerJob, error) {
	now := time.Now()
	startedAt := &now
	updated, err := s.jobStore.UpdateSubsidyAnalyzerJob(ctx, repository.UpdateSubsidyAnalyzerJobParams{
		ID:              jobID,
		Status:          status,
		Step:            step,
		ProgressPercent: progress,
		Error:           errorMsg,
		UpdatedAt:       now,
		StartedAt:       startedAt,
	})
	if err != nil {
		return nil, err
	}

	job := fromRepositorySubsidyAnalyzerJob(updated)
	s.publishJobProgress(job)
	return job, nil
}

// updateJobStatus updates the job status.

func (s *SubsidyAnalyzerService) updateJobStatus(ctx context.Context, jobID uuid.UUID, status string, step string, progress int, result map[string]interface{}, errorMsg *string) (*SubsidyAnalyzerJob, error) {
	now := time.Now()
	startedAt := &now
	updated, err := s.jobStore.UpdateSubsidyAnalyzerJob(ctx, repository.UpdateSubsidyAnalyzerJobParams{
		ID:              jobID,
		Status:          status,
		Step:            step,
		ProgressPercent: progress,
		Error:           errorMsg,
		Result:          result,
		UpdatedAt:       now,
		StartedAt:       startedAt,
		FinishedAt:      &now,
	})
	if err != nil {
		return nil, err
	}

	job := fromRepositorySubsidyAnalyzerJob(updated)
	s.publishJobProgress(job)
	return job, nil
}

// publishJobProgress publishes job progress via SSE.
func (s *SubsidyAnalyzerService) publishJobProgress(job *SubsidyAnalyzerJob) {
	if s.sseService == nil {
		return
	}
	if job == nil {
		return
	}

	data := map[string]interface{}{
		"job": map[string]interface{}{
			"jobId":           job.ID.String(),
			"status":          job.Status,
			"step":            job.Step,
			"progressPercent": job.ProgressPercent,
			"updatedAt":       job.UpdatedAt,
			"startedAt":       job.StartedAt,
		},
	}

	s.sseService.Publish(job.UserID, sse.Event{
		Type:    sse.EventSubsidyAnalysisProgress,
		Message: "Subsidy analysis progress",
		Data:    data,
	})
}

// StoreSubsidyResult saves the AI result to the job record.
func (s *SubsidyAnalyzerService) StoreSubsidyResult(ctx context.Context, jobID uuid.UUID, result map[string]interface{}) error {
	if s.jobStore == nil {
		return fmt.Errorf("subsidy analyzer job store not configured")
	}

	_, err := s.jobStore.UpdateSubsidyAnalyzerJob(ctx, repository.UpdateSubsidyAnalyzerJobParams{
		ID:              jobID,
		Status:          "completed",
		Step:            "Completed",
		ProgressPercent: 100,
		Result:          result,
		UpdatedAt:       time.Now(),
		FinishedAt:      ptr(time.Now()),
	})
	return err
}

func fromRepositorySubsidyAnalyzerJob(job repository.SubsidyAnalyzerJob) *SubsidyAnalyzerJob {
	return &SubsidyAnalyzerJob{
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
	}
}

// MarshalJSON for JSON serialization.
func (j *SubsidyAnalyzerJob) MarshalJSON() ([]byte, error) {
	type Alias SubsidyAnalyzerJob
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(j),
	})
}

// helper to create pointers
func ptr[T any](v T) *T {
	return &v
}
