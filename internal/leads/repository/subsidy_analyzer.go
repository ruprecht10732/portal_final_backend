package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"portal_final_backend/platform/apperr"
)

// SubsidyAnalyzerJob represents a subsidy analysis job record.
type SubsidyAnalyzerJob struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	UserID          uuid.UUID
	QuoteID         uuid.UUID
	Status          string // pending, running, completed, failed
	Step            string
	ProgressPercent int
	Error           *string
	Result          map[string]interface{} // ISDECalculationRequest JSON
	CreatedAt       time.Time
	StartedAt       *time.Time
	UpdatedAt       time.Time
	FinishedAt      *time.Time
}

// CreateSubsidyAnalyzerJobParams contains parameters for creating a new subsidy analyzer job.
type CreateSubsidyAnalyzerJobParams struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	UserID          uuid.UUID
	QuoteID         uuid.UUID
	Status          string // "pending"
	Step            string // "Queued"
	ProgressPercent int    // 0
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateSubsidyAnalyzerJob creates a new subsidy analyzer job.
func (r *Repository) CreateSubsidyAnalyzerJob(ctx context.Context, params CreateSubsidyAnalyzerJobParams) (SubsidyAnalyzerJob, error) {
	query := `
		INSERT INTO RAC_subsidy_analyzer_jobs (
			id, organization_id, user_id, quote_id, status, step, progress_percent,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, organization_id, user_id, quote_id, status, step, progress_percent,
		          error, result, created_at, started_at, updated_at, finished_at
	`

	row := r.pool.QueryRow(ctx, query,
		params.ID, params.OrganizationID, params.UserID, params.QuoteID,
		params.Status, params.Step, params.ProgressPercent,
		params.CreatedAt, params.UpdatedAt,
	)

	return scanSubsidyAnalyzerJob(row)
}

// GetSubsidyAnalyzerJob fetches a subsidy analyzer job by ID.
func (r *Repository) GetSubsidyAnalyzerJob(ctx context.Context, jobID uuid.UUID, organizationID uuid.UUID) (SubsidyAnalyzerJob, error) {
	query := `
		SELECT id, organization_id, user_id, quote_id, status, step, progress_percent,
		       error, result, created_at, started_at, updated_at, finished_at
		FROM RAC_subsidy_analyzer_jobs
		WHERE id = $1 AND organization_id = $2
	`

	row := r.pool.QueryRow(ctx, query, jobID, organizationID)
	job, err := scanSubsidyAnalyzerJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubsidyAnalyzerJob{}, apperr.NotFound("subsidy analyzer job not found")
	}
	return job, err
}

// UpdateSubsidyAnalyzerJobParams contains parameters for updating a subsidy analyzer job.
type UpdateSubsidyAnalyzerJobParams struct {
	ID              uuid.UUID
	Status          string
	Step            string
	ProgressPercent int
	Error           *string
	Result          map[string]interface{}
	UpdatedAt       time.Time
	FinishedAt      *time.Time
	StartedAt       *time.Time // only set once
}

// UpdateSubsidyAnalyzerJob updates an existing subsidy analyzer job.
func (r *Repository) UpdateSubsidyAnalyzerJob(ctx context.Context, params UpdateSubsidyAnalyzerJobParams) (SubsidyAnalyzerJob, error) {
	var resultJSON []byte
	if params.Result != nil {
		data, err := json.Marshal(params.Result)
		if err != nil {
			return SubsidyAnalyzerJob{}, err
		}
		resultJSON = data
	}

	query := `
		UPDATE RAC_subsidy_analyzer_jobs
		SET status = $1, step = $2, progress_percent = $3,
		    error = $4, result = $5, updated_at = $6,
		    started_at = COALESCE(started_at, $7),
		    finished_at = $8
		WHERE id = $9
		RETURNING id, organization_id, user_id, quote_id, status, step, progress_percent,
		          error, result, created_at, started_at, updated_at, finished_at
	`

	row := r.pool.QueryRow(ctx, query,
		params.Status, params.Step, params.ProgressPercent,
		params.Error, resultJSON, params.UpdatedAt,
		params.StartedAt, params.FinishedAt, params.ID,
	)

	job, err := scanSubsidyAnalyzerJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubsidyAnalyzerJob{}, apperr.NotFound("subsidy analyzer job not found")
	}
	return job, err
}

// ListSubsidyAnalyzerJobsByUser lists subsidy analyzer jobs for a user.
func (r *Repository) ListSubsidyAnalyzerJobsByUser(ctx context.Context, userID uuid.UUID, organizationID uuid.UUID, limit int, offset int) ([]SubsidyAnalyzerJob, error) {
	query := `
		SELECT id, organization_id, user_id, quote_id, status, step, progress_percent,
		       error, result, created_at, started_at, updated_at, finished_at
		FROM RAC_subsidy_analyzer_jobs
		WHERE user_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, userID, organizationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []SubsidyAnalyzerJob{}
	for rows.Next() {
		job, err := scanSubsidyAnalyzerJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// scanSubsidyAnalyzerJob scans a row into a SubsidyAnalyzerJob struct.
func scanSubsidyAnalyzerJob(row interface {
	Scan(...interface{}) error
}) (SubsidyAnalyzerJob, error) {
	var job SubsidyAnalyzerJob
	var resultJSON []byte
	var resultMap map[string]interface{}

	err := row.Scan(
		&job.ID, &job.OrganizationID, &job.UserID, &job.QuoteID,
		&job.Status, &job.Step, &job.ProgressPercent,
		&job.Error, &resultJSON,
		&job.CreatedAt, &job.StartedAt, &job.UpdatedAt, &job.FinishedAt,
	)
	if err != nil {
		return SubsidyAnalyzerJob{}, err
	}

	if len(resultJSON) > 0 {
		err = json.Unmarshal(resultJSON, &resultMap)
		if err != nil {
			return SubsidyAnalyzerJob{}, err
		}
		job.Result = resultMap
	}

	return job, nil
}
