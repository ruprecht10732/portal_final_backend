package repository

import (
	"context"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
)

const defaultPricingIntelligenceLimit = 6

type PricingIntelligenceAggregate struct {
	RegionPrefix        string
	PriceBand           string
	SampleCount         int
	AcceptedCount       int
	RejectedCount       int
	ConversionRate      float64
	AverageQuotedCents  int64
	AverageOutcomeCents *int64
}

type PricingSnapshotRecord struct {
	QuoteID       uuid.UUID
	RegionPrefix  string
	PriceBand     string
	SourceType    string
	QuoteRevision int
	TotalCents    int64
	CreatedAt     time.Time
}

type PricingOutcomeRecord struct {
	QuoteID         uuid.UUID
	RegionPrefix    string
	PriceBand       string
	OutcomeType     string
	FinalTotalCents *int64
	EstimatorRunID  *string
	Reason          *string
	CreatedAt       time.Time
}

type PricingCorrectionRecord struct {
	QuoteID         uuid.UUID
	RegionPrefix    string
	PriceBand       string
	FieldName       string
	DeltaCents      *int64
	DeltaPercentage *float64
	Reason          *string
	AIFindingCode   *string
	EstimatorRunID  *string
	CreatedAt       time.Time
}

type PricingIntelligenceReport struct {
	ServiceType       string
	RegionPrefix      string
	Aggregates        []PricingIntelligenceAggregate
	RecentSnapshots   []PricingSnapshotRecord
	RecentOutcomes    []PricingOutcomeRecord
	RecentCorrections []PricingCorrectionRecord
}

func (r *Repository) GetPricingIntelligenceReport(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string) (*PricingIntelligenceReport, error) {
	aggregates, err := r.ListPricingIntelligenceAggregates(ctx, organizationID, serviceType, postcodePrefix, defaultPricingIntelligenceLimit)
	if err != nil {
		return nil, err
	}
	snapshots, err := r.ListRecentPricingSnapshots(ctx, organizationID, serviceType, postcodePrefix, defaultPricingIntelligenceLimit)
	if err != nil {
		return nil, err
	}
	outcomes, err := r.ListRecentPricingOutcomes(ctx, organizationID, serviceType, postcodePrefix, defaultPricingIntelligenceLimit)
	if err != nil {
		return nil, err
	}
	corrections, err := r.ListRecentPricingCorrections(ctx, organizationID, serviceType, postcodePrefix, defaultPricingIntelligenceLimit)
	if err != nil {
		return nil, err
	}

	return &PricingIntelligenceReport{
		ServiceType:       serviceType,
		RegionPrefix:      postcodePrefix,
		Aggregates:        aggregates,
		RecentSnapshots:   snapshots,
		RecentOutcomes:    outcomes,
		RecentCorrections: corrections,
	}, nil
}

func (r *Repository) ListPricingIntelligenceAggregates(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string, limit int) ([]PricingIntelligenceAggregate, error) {
	if limit <= 0 {
		limit = defaultPricingIntelligenceLimit
	}
	rows, err := r.queries.ListPricingIntelligenceAggregates(ctx, quotesdb.ListPricingIntelligenceAggregatesParams{
		OrganizationID: toPgUUID(organizationID),
		ServiceType:    toPgTextPtr(&serviceType),
		Column3:        postcodePrefix,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mapPricingIntelligenceAggregates(rows), nil
}

func mapPricingIntelligenceAggregates(rows []quotesdb.ListPricingIntelligenceAggregatesRow) []PricingIntelligenceAggregate {
	items := make([]PricingIntelligenceAggregate, 0, len(rows))
	for _, row := range rows {
		var averageOutcomeCents *int64
		if row.OutcomeTotalCount > 0 {
			value := row.AverageOutcomeCents
			averageOutcomeCents = &value
		}
		items = append(items, PricingIntelligenceAggregate{
			RegionPrefix:        row.RegionPrefix,
			PriceBand:           row.PriceBand,
			SampleCount:         int(row.SampleCount),
			AcceptedCount:       int(row.AcceptedCount),
			RejectedCount:       int(row.RejectedCount),
			ConversionRate:      row.ConversionRate,
			AverageQuotedCents:  row.AverageQuotedCents,
			AverageOutcomeCents: averageOutcomeCents,
		})
	}
	return items
}

func (r *Repository) ListRecentPricingSnapshots(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string, limit int) ([]PricingSnapshotRecord, error) {
	if limit <= 0 {
		limit = defaultPricingIntelligenceLimit
	}
	rows, err := r.queries.ListRecentPricingSnapshots(ctx, quotesdb.ListRecentPricingSnapshotsParams{
		OrganizationID: toPgUUID(organizationID),
		ServiceType:    toPgTextPtr(&serviceType),
		Column3:        postcodePrefix,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]PricingSnapshotRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, PricingSnapshotRecord{
			QuoteID:       uuid.UUID(row.QuoteID.Bytes),
			RegionPrefix:  row.RegionPrefix,
			PriceBand:     row.PriceBand,
			SourceType:    row.SourceType,
			QuoteRevision: int(row.QuoteRevision),
			TotalCents:    row.TotalCents,
			CreatedAt:     timeFromPg(row.CreatedAt),
		})
	}
	return items, nil
}

func (r *Repository) ListRecentPricingOutcomes(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string, limit int) ([]PricingOutcomeRecord, error) {
	if limit <= 0 {
		limit = defaultPricingIntelligenceLimit
	}
	rows, err := r.queries.ListRecentPricingOutcomes(ctx, quotesdb.ListRecentPricingOutcomesParams{
		OrganizationID: toPgUUID(organizationID),
		ServiceType:    toPgTextPtr(&serviceType),
		Column3:        postcodePrefix,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]PricingOutcomeRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, PricingOutcomeRecord{
			QuoteID:         uuid.UUID(row.QuoteID.Bytes),
			RegionPrefix:    row.RegionPrefix,
			PriceBand:       row.PriceBand,
			OutcomeType:     row.OutcomeType,
			FinalTotalCents: optionalInt64(row.FinalTotalCents),
			EstimatorRunID:  optionalString(row.EstimatorRunID),
			Reason:          optionalString(row.RejectionReason),
			CreatedAt:       timeFromPg(row.CreatedAt),
		})
	}
	return items, nil
}

func (r *Repository) ListRecentPricingCorrections(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string, limit int) ([]PricingCorrectionRecord, error) {
	if limit <= 0 {
		limit = defaultPricingIntelligenceLimit
	}
	rows, err := r.queries.ListRecentPricingCorrections(ctx, quotesdb.ListRecentPricingCorrectionsParams{
		OrganizationID: toPgUUID(organizationID),
		ServiceType:    toPgTextPtr(&serviceType),
		Column3:        postcodePrefix,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]PricingCorrectionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, PricingCorrectionRecord{
			QuoteID:         uuid.UUID(row.QuoteID.Bytes),
			RegionPrefix:    row.RegionPrefix,
			PriceBand:       row.PriceBand,
			FieldName:       row.FieldName,
			DeltaCents:      optionalInt64(row.DeltaCents),
			DeltaPercentage: optionalFloat(row.DeltaPercentage),
			Reason:          optionalString(row.Reason),
			AIFindingCode:   optionalString(row.AiFindingCode),
			EstimatorRunID:  optionalString(row.EstimatorRunID),
			CreatedAt:       timeFromPg(row.CreatedAt),
		})
	}
	return items, nil
}
