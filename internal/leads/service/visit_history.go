package service

import (
	"context"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"

	"github.com/google/uuid"
)

func (s *Service) ListVisitHistory(ctx context.Context, leadID uuid.UUID) (transport.VisitHistoryListResponse, error) {
	// Verify lead exists
	_, err := s.repo.GetByID(ctx, leadID)
	if err != nil {
		return transport.VisitHistoryListResponse{}, ErrLeadNotFound
	}

	history, err := s.repo.ListVisitHistory(ctx, leadID)
	if err != nil {
		return transport.VisitHistoryListResponse{}, err
	}

	items := make([]transport.VisitHistoryResponse, len(history))
	for i, vh := range history {
		items[i] = toVisitHistoryResponse(vh)
	}

	return transport.VisitHistoryListResponse{Items: items}, nil
}

func (s *Service) CreateVisitHistoryEntry(ctx context.Context, params repository.CreateVisitHistoryParams) (transport.VisitHistoryResponse, error) {
	vh, err := s.repo.CreateVisitHistory(ctx, params)
	if err != nil {
		return transport.VisitHistoryResponse{}, err
	}
	return toVisitHistoryResponse(vh), nil
}

func toVisitHistoryResponse(vh repository.VisitHistory) transport.VisitHistoryResponse {
	resp := transport.VisitHistoryResponse{
		ID:            vh.ID,
		LeadID:        vh.LeadID,
		ScheduledDate: vh.ScheduledDate,
		ScoutID:       vh.ScoutID,
		Outcome:       transport.VisitOutcome(vh.Outcome),
		Measurements:  vh.Measurements,
		Notes:         vh.Notes,
		CompletedAt:   vh.CompletedAt,
		CreatedAt:     vh.CreatedAt,
	}

	if vh.AccessDifficulty != nil {
		difficulty := transport.AccessDifficulty(*vh.AccessDifficulty)
		resp.AccessDifficulty = &difficulty
	}

	return resp
}
