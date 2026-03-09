package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

type CreateCatalogSearchLogParams struct {
	OrganizationID uuid.UUID
	LeadServiceID  *uuid.UUID
	RunID          *string
	ToolName       *string
	AgentName      *string
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

	createdAt := pgtype.Timestamptz{}
	if params.CreatedAt != nil {
		createdAt = toPgTimestamp(*params.CreatedAt)
	}

	return r.queries.CreateCatalogSearchLog(ctx, leadsdb.CreateCatalogSearchLogParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		LeadServiceID:  toPgUUIDPtr(params.LeadServiceID),
		RunID:          toPgText(params.RunID),
		ToolName:       toPgText(params.ToolName),
		AgentName:      toPgText(params.AgentName),
		Query:          params.Query,
		Collection:     params.Collection,
		ResultCount:    int32(params.ResultCount),
		TopScore:       toPgFloat8Ptr(params.TopScore),
		CreatedAt:      createdAt,
	})
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

	rows, err := r.queries.ListFrequentCatalogSearchMisses(ctx, leadsdb.ListFrequentCatalogSearchMissesParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        int32(lookbackDays),
		Column3:        int64(minCount),
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("query catalog search misses: %w", err)
	}

	items := make([]CatalogSearchMissSummary, 0, len(rows))
	for _, row := range rows {
		query, _ := row.RepresentativeQuery.(string)
		lastSeen, _ := row.LastSeen.(time.Time)
		items = append(items, CatalogSearchMissSummary{
			Query:       query,
			SearchCount: int(row.Cnt),
			LastSeenAt:  lastSeen,
			Collections: row.Collections,
		})
	}

	return items, nil
}
