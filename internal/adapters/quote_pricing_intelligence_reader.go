package adapters

import (
	"context"

	leadsports "portal_final_backend/internal/leads/ports"
	quotesrepo "portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

type QuotePricingIntelligenceReader struct {
	repo *quotesrepo.Repository
}

func NewQuotePricingIntelligenceReader(repo *quotesrepo.Repository) *QuotePricingIntelligenceReader {
	return &QuotePricingIntelligenceReader{repo: repo}
}

func (a *QuotePricingIntelligenceReader) GetPricingIntelligenceReport(ctx context.Context, organizationID uuid.UUID, serviceType string, postcodePrefix string) (*leadsports.PricingIntelligenceReport, error) {
	report, err := a.repo.GetPricingIntelligenceReport(ctx, organizationID, serviceType, postcodePrefix)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}

	result := &leadsports.PricingIntelligenceReport{
		ServiceType:       report.ServiceType,
		RegionPrefix:      report.RegionPrefix,
		Aggregates:        make([]leadsports.PricingIntelligenceAggregate, 0, len(report.Aggregates)),
		RecentSnapshots:   make([]leadsports.PricingIntelligenceSnapshotRecord, 0, len(report.RecentSnapshots)),
		RecentOutcomes:    make([]leadsports.PricingIntelligenceOutcomeRecord, 0, len(report.RecentOutcomes)),
		RecentCorrections: make([]leadsports.PricingIntelligenceCorrectionRecord, 0, len(report.RecentCorrections)),
	}

	for _, aggregate := range report.Aggregates {
		result.Aggregates = append(result.Aggregates, leadsports.PricingIntelligenceAggregate{
			RegionPrefix:        aggregate.RegionPrefix,
			PriceBand:           aggregate.PriceBand,
			SampleCount:         aggregate.SampleCount,
			AcceptedCount:       aggregate.AcceptedCount,
			RejectedCount:       aggregate.RejectedCount,
			ConversionRate:      aggregate.ConversionRate,
			AverageQuotedCents:  aggregate.AverageQuotedCents,
			AverageOutcomeCents: aggregate.AverageOutcomeCents,
		})
	}
	for _, snapshot := range report.RecentSnapshots {
		result.RecentSnapshots = append(result.RecentSnapshots, leadsports.PricingIntelligenceSnapshotRecord{
			QuoteID:       snapshot.QuoteID,
			RegionPrefix:  snapshot.RegionPrefix,
			PriceBand:     snapshot.PriceBand,
			SourceType:    snapshot.SourceType,
			QuoteRevision: snapshot.QuoteRevision,
			TotalCents:    snapshot.TotalCents,
			CreatedAt:     snapshot.CreatedAt,
		})
	}
	for _, outcome := range report.RecentOutcomes {
		result.RecentOutcomes = append(result.RecentOutcomes, leadsports.PricingIntelligenceOutcomeRecord{
			QuoteID:         outcome.QuoteID,
			RegionPrefix:    outcome.RegionPrefix,
			PriceBand:       outcome.PriceBand,
			OutcomeType:     outcome.OutcomeType,
			FinalTotalCents: outcome.FinalTotalCents,
			Reason:          outcome.Reason,
			CreatedAt:       outcome.CreatedAt,
		})
	}
	for _, correction := range report.RecentCorrections {
		result.RecentCorrections = append(result.RecentCorrections, leadsports.PricingIntelligenceCorrectionRecord{
			QuoteID:         correction.QuoteID,
			RegionPrefix:    correction.RegionPrefix,
			PriceBand:       correction.PriceBand,
			FieldName:       correction.FieldName,
			DeltaCents:      correction.DeltaCents,
			DeltaPercentage: correction.DeltaPercentage,
			Reason:          correction.Reason,
			AIFindingCode:   correction.AIFindingCode,
			CreatedAt:       correction.CreatedAt,
		})
	}

	return result, nil
}

var _ leadsports.PricingIntelligenceReader = (*QuotePricingIntelligenceReader)(nil)