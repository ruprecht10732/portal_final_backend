package repository

import (
	"context"
	"fmt"
	"time"

	searchdb "portal_final_backend/internal/search/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *searchdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: searchdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

type SearchResult struct {
	ID           uuid.UUID
	Type         string
	Title        string
	Subtitle     string
	Preview      string
	Status       string
	LinkID       string
	MatchedField string
	Score        float32
	CreatedAt    time.Time
	Total        int64
}

func (r *Repository) GlobalSearch(ctx context.Context, orgID uuid.UUID, query string, limit int, types []string) ([]SearchResult, error) {
	typesArg := types
	if len(types) > 0 {
		typesArg = types
	} else {
		typesArg = nil
	}

	rows, err := r.queries.GlobalSearch(ctx, searchdb.GlobalSearchParams{
		Types:      typesArg,
		LimitCount: int32(limit),
		QueryText:  query,
		OrgID:      toPgUUID(orgID),
	})
	if err != nil {
		return nil, fmt.Errorf("fts global search query failed: %w", err)
	}

	items := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, SearchResult{
			ID:           uuid.UUID(row.ID.Bytes),
			Type:         row.Type,
			Title:        row.Title,
			Subtitle:     row.Subtitle,
			Preview:      row.Preview,
			Status:       row.Status,
			LinkID:       row.LinkID,
			MatchedField: row.MatchedField,
			Score:        row.Score,
			CreatedAt:    row.CreatedAt.Time,
			Total:        row.Total,
		})
	}

	return items, nil
}
