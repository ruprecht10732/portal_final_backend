package repository

import (
	"context"

	"github.com/google/uuid"

	leadsdb "portal_final_backend/internal/leads/db"
)

const dashboardTrendWeeks = 6

// LeadMetrics aggregates KPI values for the dashboard.
type LeadMetrics struct {
	ActiveLeads         int
	AcceptedQuotes      int
	SentQuotes          int
	QuotePipelineCents  int64
	AvgQuoteValueCents  int64
	ActiveLeadsTrend    []int
	AcceptedQuotesTrend []int
	SentQuotesTrend     []int
	QuotePipelineTrend  []int64
	AvgQuoteValueTrend  []int64
}

// GetMetrics returns KPI aggregates for active (non-deleted) RAC_leads within an organization.
func (r *Repository) GetMetrics(ctx context.Context, organizationID uuid.UUID) (LeadMetrics, error) {
	summary, err := r.queries.GetLeadMetricsSummary(ctx, toPgUUID(organizationID))
	if err != nil {
		return LeadMetrics{}, err
	}

	metrics := LeadMetrics{
		ActiveLeads:        int(summary.ActiveLeads),
		AcceptedQuotes:     int(summary.AcceptedQuotes),
		SentQuotes:         int(summary.SentQuotes),
		QuotePipelineCents: summary.QuotePipelineCents,
		AvgQuoteValueCents: summary.AvgQuoteValueCents,
	}

	activeTrend, err := r.listActiveLeadsTrend(ctx, organizationID)
	if err != nil {
		return LeadMetrics{}, err
	}

	acceptedQuotesTrend, sentQuotesTrend, err := r.listQuoteOutcomeTrend(ctx, organizationID)
	if err != nil {
		return LeadMetrics{}, err
	}

	quotePipelineTrend, err := r.listQuotePipelineTrend(ctx, organizationID)
	if err != nil {
		return LeadMetrics{}, err
	}

	avgQuoteValueTrend, err := r.listAvgQuoteValueTrend(ctx, organizationID)
	if err != nil {
		return LeadMetrics{}, err
	}

	metrics.ActiveLeadsTrend = activeTrend
	metrics.AcceptedQuotesTrend = acceptedQuotesTrend
	metrics.SentQuotesTrend = sentQuotesTrend
	metrics.QuotePipelineTrend = quotePipelineTrend
	metrics.AvgQuoteValueTrend = avgQuoteValueTrend

	return metrics, nil
}

func (r *Repository) listActiveLeadsTrend(ctx context.Context, organizationID uuid.UUID) ([]int, error) {
	rows, err := r.queries.ListActiveLeadsTrend(ctx, leadsdb.ListActiveLeadsTrendParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        dashboardTrendWeeks,
	})
	if err != nil {
		return nil, err
	}
	activeTrend := make([]int, 0, dashboardTrendWeeks)
	for _, value := range rows {
		activeTrend = append(activeTrend, int(value))
	}

	return padIntTrend(activeTrend), nil
}

func (r *Repository) listQuoteOutcomeTrend(ctx context.Context, organizationID uuid.UUID) ([]int, []int, error) {
	rows, err := r.queries.ListQuoteOutcomeTrend(ctx, leadsdb.ListQuoteOutcomeTrendParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        dashboardTrendWeeks,
	})
	if err != nil {
		return nil, nil, err
	}

	acceptedTrend := make([]int, 0, dashboardTrendWeeks)
	sentTrend := make([]int, 0, dashboardTrendWeeks)
	for _, row := range rows {
		acceptedTrend = append(acceptedTrend, int(row.AcceptedQuotes))
		sentTrend = append(sentTrend, int(row.SentQuotes))
	}

	return padIntTrend(acceptedTrend), padIntTrend(sentTrend), nil
}

func (r *Repository) listQuotePipelineTrend(ctx context.Context, organizationID uuid.UUID) ([]int64, error) {
	rows, err := r.queries.ListQuotePipelineTrend(ctx, leadsdb.ListQuotePipelineTrendParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        dashboardTrendWeeks,
	})
	if err != nil {
		return nil, err
	}

	trend := make([]int64, 0, dashboardTrendWeeks)
	for _, value := range rows {
		trend = append(trend, value)
	}

	return padInt64Trend(trend), nil
}

func (r *Repository) listAvgQuoteValueTrend(ctx context.Context, organizationID uuid.UUID) ([]int64, error) {
	rows, err := r.queries.ListAvgQuoteValueTrend(ctx, leadsdb.ListAvgQuoteValueTrendParams{
		OrganizationID: toPgUUID(organizationID),
		Column2:        dashboardTrendWeeks,
	})
	if err != nil {
		return nil, err
	}

	trend := make([]int64, 0, dashboardTrendWeeks)
	for _, value := range rows {
		trend = append(trend, value)
	}

	return padInt64Trend(trend), nil
}

func padIntTrend(values []int) []int {
	if len(values) >= dashboardTrendWeeks {
		return values
	}
	result := make([]int, dashboardTrendWeeks)
	copy(result[dashboardTrendWeeks-len(values):], values)
	return result
}

func padInt64Trend(values []int64) []int64 {
	if len(values) >= dashboardTrendWeeks {
		return values
	}
	result := make([]int64, dashboardTrendWeeks)
	copy(result[dashboardTrendWeeks-len(values):], values)
	return result
}
