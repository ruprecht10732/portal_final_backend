package service

import (
	"context"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

// ListGenerateQuoteJobs lists async quote generation jobs for the current user.
func (s *Service) ListGenerateQuoteJobs(ctx context.Context, tenantID, userID uuid.UUID, page, limit int) ([]GenerateQuoteJob, int, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, 0, apperr.Validation("tenantId and userId are required")
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

	rows, total, err := s.repo.ListGenerateQuoteJobs(ctx, tenantID, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	result := make([]GenerateQuoteJob, 0, len(rows))
	for i := range rows {
		mapped := repositoryJobToServiceJob(&rows[i])
		if mapped == nil {
			continue
		}
		result = append(result, *mapped)
	}

	return result, total, nil
}

// DeleteGenerateQuoteJob deletes a finished (completed/failed) job for the current user.
// Active jobs cannot be deleted.
func (s *Service) DeleteGenerateQuoteJob(ctx context.Context, tenantID, userID, jobID uuid.UUID) error {
	if tenantID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return apperr.Validation("tenantId, userId and jobId are required")
	}

	current, err := s.GetGenerateQuoteJob(ctx, tenantID, userID, jobID)
	if err != nil {
		return err
	}

	if current.Status == GenerateQuoteJobStatusPending || current.Status == GenerateQuoteJobStatusRunning {
		return apperr.Conflict("cannot delete an active job")
	}

	return s.repo.DeleteGenerateQuoteJob(ctx, tenantID, userID, jobID)
}

// ClearCompletedGenerateQuoteJobs deletes all completed jobs for the current user.
func (s *Service) ClearCompletedGenerateQuoteJobs(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return 0, apperr.Validation("tenantId and userId are required")
	}

	return s.repo.DeleteCompletedGenerateQuoteJobs(ctx, tenantID, userID)
}
