package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/search/repository"
	"portal_final_backend/internal/search/transport"
	"portal_final_backend/platform/apperr"
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GlobalSearch(ctx context.Context, orgID uuid.UUID, req transport.SearchRequest) (*transport.SearchResponse, error) {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return &transport.SearchResponse{Items: []transport.SearchResultItem{}, Total: 0}, nil
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	results, err := s.repo.GlobalSearch(ctx, orgID, q, limit)
	if err != nil {
		appErr := apperr.Internal("search failed").WithOp("search.GlobalSearch").WithDetails(err.Error())
		appErr.Err = err
		return nil, appErr
	}

	total := 0
	if len(results) > 0 {
		// COUNT(*) OVER() returns bigint
		if results[0].Total > 0 {
			if results[0].Total > int64(^uint(0)>>1) {
				total = int(^uint(0) >> 1)
			} else {
				total = int(results[0].Total)
			}
		}
	}

	items := make([]transport.SearchResultItem, len(results))
	for i, r := range results {
		items[i] = transport.SearchResultItem{
			ID:           r.ID.String(),
			Type:         r.Type,
			Title:        r.Title,
			Subtitle:     r.Subtitle,
			Preview:      r.Preview,
			Status:       r.Status,
			Link:         buildFrontendLink(r.Type, r.LinkID),
			Score:        float64(r.Score),
			MatchedField: r.MatchedField,
			CreatedAt:    r.CreatedAt,
		}
	}

	return &transport.SearchResponse{Items: items, Total: total}, nil
}

func buildFrontendLink(entityType, linkID string) string {
	switch entityType {
	case "lead":
		return "/app/leads/" + linkID
	case "quote":
		return "/app/offertes/" + linkID
	case "partner":
		return "/app/partners/" + linkID
	case "appointment":
		return "/app/appointments/" + linkID
	default:
		return "/app"
	}
}
