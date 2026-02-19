package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CreateCatalogSearchLogParams struct {
	OrganizationID uuid.UUID
	LeadServiceID  *uuid.UUID
	Query          string
	Collection     string
	ResultCount    int
	TopScore       *float64
	CreatedAt      *time.Time // optional override (normally server-side now())
}

// CatalogSearchMissSummary aggregates frequent catalog searches with 0 results.
type CatalogSearchMissSummary struct {
	Query       string
	SearchCount int
	LastSeenAt  time.Time
	Collections []string
}

func (r *Repository) CreateCatalogSearchLog(ctx context.Context, params CreateCatalogSearchLogParams) error {
	if params.OrganizationID == uuid.Nil {
		return fmt.Errorf("organization_id is required")
	}
	if params.Query == "" {
		return fmt.Errorf("query is required")
	}
	if params.Collection == "" {
		return fmt.Errorf("collection is required")
	}
	if params.ResultCount < 0 {
		return fmt.Errorf("result_count cannot be negative")
	}

	// Prefer DB-side now() unless an explicit timestamp is provided.
	if params.CreatedAt == nil {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO RAC_catalog_search_log (organization_id, lead_service_id, query, collection, result_count, top_score)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, params.OrganizationID, params.LeadServiceID, params.Query, params.Collection, params.ResultCount, params.TopScore)
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO RAC_catalog_search_log (organization_id, lead_service_id, query, collection, result_count, top_score, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, params.OrganizationID, params.LeadServiceID, params.Query, params.Collection, params.ResultCount, params.TopScore, *params.CreatedAt)
	return err
}

// ListFrequentCatalogSearchMisses returns distinct query strings that repeatedly
// produced 0 results within the lookback window.
func (r *Repository) ListFrequentCatalogSearchMisses(ctx context.Context, organizationID uuid.UUID, lookbackDays int, minCount int, limit int) ([]CatalogSearchMissSummary, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if lookbackDays <= 0 {
		lookbackDays = 14
	}
	if minCount <= 0 {
		minCount = 3
	}
	if limit <= 0 {
		limit = 25
	}

	// Normalize in SQL to reduce trivial duplicates (case, whitespace).
	// Keep the original "representative" query via min(query) for human review.
	rows, err := r.pool.Query(ctx, `
		WITH misses AS (
			SELECT
				LOWER(REGEXP_REPLACE(TRIM(query), '\\s+', ' ', 'g')) AS qnorm,
				query,
				collection,
				created_at
			FROM RAC_catalog_search_log
			WHERE organization_id = $1
				AND result_count = 0
				AND created_at >= (NOW() - ($2::int || ' days')::interval)
		)
		SELECT
			MIN(query) AS representative_query,
			COUNT(*)::int AS cnt,
			MAX(created_at) AS last_seen,
			ARRAY_AGG(DISTINCT collection) AS collections
		FROM misses
		GROUP BY qnorm
		HAVING COUNT(*) >= $3
		ORDER BY cnt DESC, last_seen DESC
		LIMIT $4
	`, organizationID, lookbackDays, minCount, limit)
	if err != nil {
		return nil, fmt.Errorf("query catalog search misses: %w", err)
	}
	defer rows.Close()

	items := make([]CatalogSearchMissSummary, 0)
	for rows.Next() {
		var it CatalogSearchMissSummary
		if err := rows.Scan(&it.Query, &it.SearchCount, &it.LastSeenAt, &it.Collections); err != nil {
			return nil, fmt.Errorf("scan catalog search miss summary: %w", err)
		}
		items = append(items, it)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate catalog search miss summaries: %w", rows.Err())
	}

	return items, nil
}
