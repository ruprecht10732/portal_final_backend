package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/quotes/repository"
)

func (s *Service) RecordQuoteAIReview(ctx context.Context, params RecordQuoteAIReviewParams) (*QuoteAIReviewResult, error) {
	findings, err := json.Marshal(params.Findings)
	if err != nil {
		return nil, fmt.Errorf("marshal quote ai review findings: %w", err)
	}
	signals, err := json.Marshal(params.Signals)
	if err != nil {
		return nil, fmt.Errorf("marshal quote ai review signals: %w", err)
	}

	createdAt := time.Now()
	review, err := s.repo.CreateQuoteAIReview(ctx, repository.CreateQuoteAIReviewParams{
		ID:             uuid.New(),
		OrganizationID: params.OrganizationID,
		QuoteID:        params.QuoteID,
		Decision:       params.Decision,
		Summary:        params.Summary,
		Findings:       findings,
		Signals:        signals,
		AttemptCount:   params.AttemptCount,
		RunID:          params.RunID,
		ReviewerName:   params.ReviewerName,
		ModelName:      params.ModelName,
		CreatedAt:      createdAt,
	})
	if err != nil {
		return nil, err
	}

	metadata, _ := json.Marshal(map[string]any{
		"reviewId":      review.ID,
		"decision":      review.Decision,
		"attemptCount":  review.AttemptCount,
		"findingsCount": len(params.Findings),
		"signals":       params.Signals,
	})
	_ = s.repo.CreateActivity(ctx, &repository.QuoteActivity{
		ID:             uuid.New(),
		QuoteID:        params.QuoteID,
		OrganizationID: params.OrganizationID,
		EventType:      "ai_review",
		Message:        params.Summary,
		Metadata:       metadata,
		CreatedAt:      createdAt,
	})

	return &QuoteAIReviewResult{
		ReviewID:     review.ID,
		QuoteID:      review.QuoteID,
		Decision:     review.Decision,
		Summary:      review.Summary,
		AttemptCount: review.AttemptCount,
		CreatedAt:    review.CreatedAt,
	}, nil
}