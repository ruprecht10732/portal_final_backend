package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	leadagent "portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	quoterepo "portal_final_backend/internal/quotes/repository"
	quotetransport "portal_final_backend/internal/quotes/transport"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/logger"
)

const (
	errMsgFailedToUpdateJobProgress = "failed to update job progress"
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

// SubsidyAnalyzerService handles subsidy analysis job orchestration.
type SubsidyAnalyzerService struct {
	repo            repository.LeadsRepository
	quoteRepo       *quoterepo.Repository
	analyzer        *leadagent.SubsidyAnalyzer
	eventBus        events.Bus
	sseService      *sse.Service
	schedulerClient *scheduler.Client
	log             *logger.Logger
	moonshotAPIKey  string
	llmModel        string

	// In-memory job store (temporary; should be persisted to DB)
	jobsMu sync.RWMutex
	jobs   map[uuid.UUID]*SubsidyAnalyzerJob
}

// SubsidyAnalyzerServiceConfig holds dependencies for the service.
type SubsidyAnalyzerServiceConfig struct {
	Repo            repository.LeadsRepository
	QuoteRepo       *quoterepo.Repository
	EventBus        events.Bus
	SSEService      *sse.Service
	SchedulerClient *scheduler.Client
	Log             *logger.Logger
	MoonshotAPIKey  string
	LLMModel        string
}

// NewSubsidyAnalyzerService creates a new subsidy analyzer service.
func NewSubsidyAnalyzerService(cfg SubsidyAnalyzerServiceConfig) *SubsidyAnalyzerService {
	return &SubsidyAnalyzerService{
		repo:            cfg.Repo,
		quoteRepo:       cfg.QuoteRepo,
		analyzer:        nil,
		eventBus:        cfg.EventBus,
		sseService:      cfg.SSEService,
		schedulerClient: cfg.SchedulerClient,
		log:             cfg.Log,
		moonshotAPIKey:  cfg.MoonshotAPIKey,
		llmModel:        cfg.LLMModel,
		jobs:            make(map[uuid.UUID]*SubsidyAnalyzerJob),
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

func (s *SubsidyAnalyzerService) ensureAnalyzer() (*leadagent.SubsidyAnalyzer, error) {
	if s.analyzer != nil {
		return s.analyzer, nil
	}
	if s.moonshotAPIKey == "" {
		return nil, fmt.Errorf("moonshot api key not configured for subsidy analyzer")
	}

	analyzer, err := leadagent.NewSubsidyAnalyzerAgent(leadagent.SubsidyAnalyzerConfig{
		APIKey: s.moonshotAPIKey,
		Model:  s.llmModel,
		Repo:   s.repo,
	})
	if err != nil {
		return nil, err
	}

	s.analyzer = analyzer
	return s.analyzer, nil
}

func buildDraftQuoteContext(items []quotetransport.QuoteItemRequest) string {
	var builder strings.Builder
	builder.WriteString("Quote line items:\n")
	for index, item := range items {
		builder.WriteString(fmt.Sprintf("%d. ", index+1))
		if item.Title != "" {
			builder.WriteString(item.Title)
			builder.WriteString(" - ")
		}
		builder.WriteString(item.Description)
		builder.WriteString(fmt.Sprintf(" | quantity: %s", item.Quantity))
		if item.IsOptional {
			builder.WriteString(" | optional")
		}
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
	jobID := uuid.New()
	now := time.Now()

	// Create job record in memory
	job := &SubsidyAnalyzerJob{
		ID:              jobID,
		OrganizationID:  organizationID,
		UserID:          userID,
		QuoteID:         quoteID,
		Status:          "pending",
		Step:            "Queued",
		ProgressPercent: 0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	s.jobsMu.Lock()
	s.jobs[jobID] = job
	s.jobsMu.Unlock()

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

	err := s.schedulerClient.EnqueueSubsidyAnalyzerJob(ctx, taskPayload)
	if err != nil {
		s.updateJobStatus(jobID, "failed", "Queueing failed", ptr(err.Error()))
		return uuid.Nil, fmt.Errorf("failed to enqueue subsidy analyzer task: %w", err)
	}

	return jobID, nil
}

// GetSubsidyAnalysisJob fetches the current status of a job.
func (s *SubsidyAnalyzerService) GetSubsidyAnalysisJob(ctx context.Context, jobID uuid.UUID, organizationID uuid.UUID) (interface{}, error) {
	s.jobsMu.RLock()
	job, exists := s.jobs[jobID]
	s.jobsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	if job.OrganizationID != organizationID {
		return nil, fmt.Errorf("unauthorized: job not in organization")
	}

	return job, nil
}

// ProcessSubsidyAnalysisJob is the main orchestration logic for analyzing subsidies.
// This is called by the scheduler worker.
func (s *SubsidyAnalyzerService) ProcessSubsidyAnalysisJob(ctx context.Context, jobID uuid.UUID, quoteID uuid.UUID, organizationID uuid.UUID) error {
	// Mark as running
	s.updateJobProgress(jobID, "running", "Loading quote context", 10, nil)

	// Step 1: Log job start (actual quote loading happens in the agent)
	if s.log != nil {
		s.log.Info("subsidy analysis job started", "jobId", jobID.String(), "quoteId", quoteID.String())
	}

	// Step 2: Load ISDE measures and rules
	s.updateJobProgress(jobID, "running", "Loading ISDE rules", 30, nil)

	// Step 3: Analyze with AI
	s.updateJobProgress(jobID, "running", "Analyzing with AI", 50, nil)

	// NOTE: The actual LLM call happens in the ADK agent (subsidy_analyzer.go in agent package)
	// This service prepares the job record and enqueues it; the agent does the thinking

	// Step 4: Mark as completed
	s.updateJobProgress(jobID, "running", "Finalizing result", 90, nil)
	s.updateJobStatus(jobID, "completed", "Completed", nil)

	return nil
}

// updateJobProgress updates job status and emits SSE event.
func (s *SubsidyAnalyzerService) updateJobProgress(jobID uuid.UUID, status string, step string, progress int, errorMsg *string) {
	s.jobsMu.Lock()
	job, exists := s.jobs[jobID]
	s.jobsMu.Unlock()

	if !exists {
		return
	}

	now := time.Now()
	if job.StartedAt == nil {
		job.StartedAt = &now
	}

	job.Status = status
	job.Step = step
	job.ProgressPercent = progress
	job.Error = errorMsg
	job.UpdatedAt = now

	s.publishJobProgress(jobID, status, step, progress)
}

// updateJobStatus updates the job status.
func (s *SubsidyAnalyzerService) updateJobStatus(jobID uuid.UUID, status string, step string, errorMsg *string) {
	s.jobsMu.Lock()
	job, exists := s.jobs[jobID]
	s.jobsMu.Unlock()

	if !exists {
		return
	}

	now := time.Now()
	job.Status = status
	job.Step = step
	job.Error = errorMsg
	job.UpdatedAt = now
	job.FinishedAt = &now

	s.publishJobProgress(jobID, status, step, job.ProgressPercent)
}

// failJob marks a job as failed.
func (s *SubsidyAnalyzerService) failJob(jobID uuid.UUID, step string, err error) error {
	errMsg := err.Error()
	s.updateJobStatus(jobID, "failed", step, &errMsg)
	return err
}

// publishJobProgress publishes job progress via SSE.
func (s *SubsidyAnalyzerService) publishJobProgress(jobID uuid.UUID, status string, step string, progress int) {
	if s.sseService == nil {
		return
	}

	s.jobsMu.RLock()
	job, exists := s.jobs[jobID]
	s.jobsMu.RUnlock()
	if !exists {
		return
	}

	data := map[string]interface{}{
		"job": map[string]interface{}{
			"jobId":           jobID.String(),
			"status":          status,
			"step":            step,
			"progressPercent": progress,
			"updatedAt":       job.UpdatedAt,
			"startedAt":       job.CreatedAt,
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
	s.jobsMu.Lock()
	job, exists := s.jobs[jobID]
	s.jobsMu.Unlock()

	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.Result = result
	job.UpdatedAt = time.Now()
	return nil
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
