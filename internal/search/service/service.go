package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/search/repository"
	"portal_final_backend/internal/search/transport"
	"portal_final_backend/platform/apperr"
)

var allowedSearchTypes = map[string]struct{}{
	"lead":            {},
	"quote":           {},
	"partner":         {},
	"appointment":     {},
	"catalog_product": {},
	"service_type":    {},
}

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GlobalSearch(ctx context.Context, orgID uuid.UUID, req transport.SearchRequest, isAdmin bool) (*transport.SearchResponse, error) {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return &transport.SearchResponse{Items: []transport.SearchResultItem{}, Total: 0}, nil
	}

	types, err := parseSearchTypes(req.Types)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		types = restrictTypesForNonAdmin(types)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	results, err := s.repo.GlobalSearch(ctx, orgID, q, limit, types)
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

func restrictTypesForNonAdmin(types []string) []string {
	if len(types) == 0 {
		return []string{"lead", "quote", "partner", "appointment", "catalog_product"}
	}

	filtered := make([]string, 0, len(types))
	for _, t := range types {
		if t == "service_type" {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
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
	case "catalog_product":
		return "/app/catalog/" + linkID
	case "service_type":
		return "/app/services/" + linkID
	default:
		return "/app"
	}
}

func parseSearchTypes(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := allowedSearchTypes[t]; !ok {
			return nil, apperr.BadRequest("invalid search type").WithDetails("unsupported type: " + t)
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		result = append(result, t)
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
