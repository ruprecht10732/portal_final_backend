package service

import (
	"context"
	"strings"
	"time"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const generateQuoteJobIdentityRequiredMsg = "tenantId, userId and jobId are required"

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

// CancelGenerateQuoteJob cancels an active job for the current user.
func (s *Service) CancelGenerateQuoteJob(ctx context.Context, tenantID, userID, jobID uuid.UUID) (*GenerateQuoteJob, error) {
	return s.CancelGenerateQuoteJobWithReason(ctx, tenantID, userID, jobID, nil)
}

// CancelGenerateQuoteJobWithReason cancels an active job and records an optional user-provided reason.
func (s *Service) CancelGenerateQuoteJobWithReason(ctx context.Context, tenantID, userID, jobID uuid.UUID, reason *string) (*GenerateQuoteJob, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return nil, apperr.Validation(generateQuoteJobIdentityRequiredMsg)
	}

	current, err := s.GetGenerateQuoteJob(ctx, tenantID, userID, jobID)
	if err != nil {
		return nil, err
	}

	if current.Status == GenerateQuoteJobStatusCompleted || current.Status == GenerateQuoteJobStatusFailed || current.Status == GenerateQuoteJobStatusCancelled {
		return nil, apperr.Conflict("cannot cancel a finished job")
	}

	now := time.Now()
	job, err := s.repo.CancelGenerateQuoteJob(ctx, tenantID, userID, jobID, now, now, normalizeOptionalText(reason))
	if err != nil {
		return nil, err
	}

	mapped := repositoryJobToServiceJob(job)
	s.publishJobProgress(mapped)
	return mapped, nil
}

// SubmitGenerateQuoteJobFeedback records lightweight user feedback for a finished job.
func (s *Service) SubmitGenerateQuoteJobFeedback(ctx context.Context, tenantID, userID, jobID uuid.UUID, rating int, comment string) (*GenerateQuoteJob, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return nil, apperr.Validation(generateQuoteJobIdentityRequiredMsg)
	}
	if rating != -1 && rating != 1 {
		return nil, apperr.Validation("rating must be -1 or 1")
	}
	job, err := s.repo.SubmitGenerateQuoteJobFeedback(ctx, tenantID, userID, jobID, rating, normalizeOptionalText(&comment), time.Now())
	if err != nil {
		return nil, err
	}
	mapped := repositoryJobToServiceJob(job)
	s.publishJobProgress(mapped)
	return mapped, nil
}

// MarkGenerateQuoteJobViewed records that the user has opened a completed job result.
func (s *Service) MarkGenerateQuoteJobViewed(ctx context.Context, tenantID, userID, jobID uuid.UUID) (*GenerateQuoteJob, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return nil, apperr.Validation(generateQuoteJobIdentityRequiredMsg)
	}
	job, err := s.repo.MarkGenerateQuoteJobViewed(ctx, tenantID, userID, jobID, time.Now())
	if err != nil {
		return nil, err
	}
	mapped := repositoryJobToServiceJob(job)
	s.publishJobProgress(mapped)
	return mapped, nil
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// DeleteGenerateQuoteJob deletes a finished (completed/failed/cancelled) job for the current user.
// Active jobs cannot be deleted.
func (s *Service) DeleteGenerateQuoteJob(ctx context.Context, tenantID, userID, jobID uuid.UUID) error {
	if tenantID == uuid.Nil || userID == uuid.Nil || jobID == uuid.Nil {
		return apperr.Validation(generateQuoteJobIdentityRequiredMsg)
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
