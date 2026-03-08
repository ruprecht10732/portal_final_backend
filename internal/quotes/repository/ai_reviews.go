package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type QuoteAIReview struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	QuoteID        uuid.UUID
	Decision       string
	Summary        string
	Findings       []byte
	Signals        []byte
	AttemptCount   int
	RunID          *string
	ReviewerName   *string
	ModelName      *string
	CreatedAt      time.Time
}

type CreateQuoteAIReviewParams struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	QuoteID        uuid.UUID
	Decision       string
	Summary        string
	Findings       []byte
	Signals        []byte
	AttemptCount   int
	RunID          *string
	ReviewerName   *string
	ModelName      *string
	CreatedAt      time.Time
}

func (r *Repository) CreateQuoteAIReview(ctx context.Context, params CreateQuoteAIReviewParams) (*QuoteAIReview, error) {
	row, err := r.queries.CreateQuoteAIReview(ctx, quotesdb.CreateQuoteAIReviewParams{
		ID:             toPgUUID(params.ID),
		OrganizationID: toPgUUID(params.OrganizationID),
		QuoteID:        toPgUUID(params.QuoteID),
		Decision:       params.Decision,
		Summary:        params.Summary,
		Findings:       params.Findings,
		Signals:        params.Signals,
		AttemptCount:   int32(params.AttemptCount),
		RunID:          toPgTextPtr(params.RunID),
		ReviewerName:   toPgTextPtr(params.ReviewerName),
		ModelName:      toPgTextPtr(params.ModelName),
		CreatedAt:      toPgTimestamp(params.CreatedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create quote ai review: %w", err)
	}
	review := quoteAIReviewFromModel(row)
	return &review, nil
}

func (r *Repository) GetLatestQuoteAIReview(ctx context.Context, quoteID, orgID uuid.UUID) (*QuoteAIReview, error) {
	row, err := r.queries.GetLatestQuoteAIReview(ctx, quotesdb.GetLatestQuoteAIReviewParams{
		QuoteID:        toPgUUID(quoteID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("quote ai review not found")
		}
		return nil, fmt.Errorf("get latest quote ai review: %w", err)
	}
	review := quoteAIReviewFromModel(row)
	return &review, nil
}

func quoteAIReviewFromModel(row quotesdb.RacQuoteAiReview) QuoteAIReview {
	return QuoteAIReview{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		QuoteID:        uuid.UUID(row.QuoteID.Bytes),
		Decision:       row.Decision,
		Summary:        row.Summary,
		Findings:       row.Findings,
		Signals:        row.Signals,
		AttemptCount:   int(row.AttemptCount),
		RunID:          optionalString(row.RunID),
		ReviewerName:   optionalString(row.ReviewerName),
		ModelName:      optionalString(row.ModelName),
		CreatedAt:      row.CreatedAt.Time,
	}
}
