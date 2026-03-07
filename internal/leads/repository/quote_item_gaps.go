package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	leadsdb "portal_final_backend/internal/leads/db"
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

	rows, err := r.queries.ListFrequentAdHocQuoteItems(ctx, leadsdb.ListFrequentAdHocQuoteItemsParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        int32(lookbackDays),
		Column3:        int64(minCount),
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("query ad-hoc quote item summaries: %w", err)
	}

	items := make([]AdHocQuoteItemSummary, 0, len(rows))
	for _, row := range rows {
		description, _ := row.RepresentativeDescription.(string)
		lastSeen, _ := row.LastSeen.(time.Time)
		items = append(items, AdHocQuoteItemSummary{
			Description: description,
			UseCount:    int(row.Cnt),
			LastSeenAt:  lastSeen,
		})
	}

	return items, nil
}
