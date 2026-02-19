package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AdHocQuoteItemSummary aggregates frequently used ad-hoc quote line items.
type AdHocQuoteItemSummary struct {
	Description string
	UseCount    int
	LastSeenAt  time.Time
}

// ListFrequentAdHocQuoteItems returns commonly used quote item descriptions where
// catalog_product_id is NULL (ad-hoc items), within the lookback window.
func (r *Repository) ListFrequentAdHocQuoteItems(ctx context.Context, organizationID uuid.UUID, lookbackDays int, minCount int, limit int) ([]AdHocQuoteItemSummary, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if lookbackDays <= 0 {
		lookbackDays = 30
	}
	if minCount <= 0 {
		minCount = 3
	}
	if limit <= 0 {
		limit = 25
	}

	rows, err := r.pool.Query(ctx, `
		WITH items AS (
			SELECT
				LOWER(REGEXP_REPLACE(TRIM(qi.description), '\\s+', ' ', 'g')) AS dnorm,
				qi.description,
				q.created_at
			FROM RAC_quote_items qi
			JOIN RAC_quotes q ON q.id = qi.quote_id
			WHERE qi.organization_id = $1
				AND q.organization_id = $1
				AND q.status != 'Draft'
				AND qi.catalog_product_id IS NULL
				AND qi.description IS NOT NULL
				AND TRIM(qi.description) != ''
				AND q.created_at >= (NOW() - ($2::int || ' days')::interval)
				AND (qi.is_optional = false OR qi.is_selected = true)
		)
		SELECT
			MIN(description) AS representative_description,
			COUNT(*)::int AS cnt,
			MAX(created_at) AS last_seen
		FROM items
		GROUP BY dnorm
		HAVING COUNT(*) >= $3
		ORDER BY cnt DESC, last_seen DESC
		LIMIT $4
	`, organizationID, lookbackDays, minCount, limit)
	if err != nil {
		return nil, fmt.Errorf("query ad-hoc quote item summaries: %w", err)
	}
	defer rows.Close()

	items := make([]AdHocQuoteItemSummary, 0)
	for rows.Next() {
		var it AdHocQuoteItemSummary
		if err := rows.Scan(&it.Description, &it.UseCount, &it.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan ad-hoc quote item summary: %w", err)
		}
		items = append(items, it)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate ad-hoc quote item summaries: %w", rows.Err())
	}

	return items, nil
}
