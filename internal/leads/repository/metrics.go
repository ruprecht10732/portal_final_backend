package repository

import (
	"context"

	"github.com/google/uuid"
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
	var metrics LeadMetrics
	err := r.pool.QueryRow(ctx, `
		SELECT
			-- Active leads (not in terminal pipeline stage, and not disqualified)
			(
				SELECT COUNT(DISTINCT l.id)
				FROM RAC_leads l
				JOIN RAC_lead_services ls ON ls.lead_id = l.id
				WHERE l.organization_id = $1 AND l.deleted_at IS NULL
					AND ls.pipeline_stage NOT IN ('Completed', 'Lost')
					AND ls.status != 'Disqualified'
			) AS active_leads,
			-- Accepted quotes count
			(
				SELECT COUNT(*)
				FROM RAC_quotes q
				WHERE q.organization_id = $1
					AND q.status::text IN ('Accepted', 'Quote_Accepted')
			) AS accepted_quotes,
			-- Sent quotes count
			(
				SELECT COUNT(*)
				FROM RAC_quotes q
				WHERE q.organization_id = $1
					AND q.status::text IN ('Sent', 'Quote_Sent')
			) AS sent_quotes,
			-- Total value of Sent and Accepted quotes (pipeline)
			(
				SELECT COALESCE(SUM(q.total_cents), 0)
				FROM RAC_quotes q
				WHERE q.organization_id = $1
					AND q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted')
			) AS quote_pipeline_cents,
			-- Average quote value for Sent and Accepted quotes
			(
				SELECT COALESCE(AVG(q.total_cents)::bigint, 0)
				FROM RAC_quotes q
				WHERE q.organization_id = $1
					AND q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted')
			) AS avg_quote_value_cents
	`, organizationID).Scan(
		&metrics.ActiveLeads,
		&metrics.AcceptedQuotes,
		&metrics.SentQuotes,
		&metrics.QuotePipelineCents,
		&metrics.AvgQuoteValueCents,
	)
	if err != nil {
		return LeadMetrics{}, err
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
	rows, err := r.pool.Query(ctx, `
		WITH weeks AS (
			SELECT generate_series(
				date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
				date_trunc('week', NOW()),
				INTERVAL '1 week'
			) AS week_start
		)
		SELECT
			w.week_start,
			COALESCE(COUNT(DISTINCT CASE WHEN ls.pipeline_stage NOT IN ('Completed', 'Lost') AND ls.status != 'Disqualified' THEN l.id END), 0) AS active_leads
		FROM weeks w
		LEFT JOIN RAC_leads l
			ON l.organization_id = $1
			AND l.deleted_at IS NULL
			AND l.created_at >= w.week_start
			AND l.created_at < w.week_start + INTERVAL '1 week'
		LEFT JOIN RAC_lead_services ls ON ls.lead_id = l.id
		GROUP BY w.week_start
		ORDER BY w.week_start
	`, organizationID, dashboardTrendWeeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	activeTrend := make([]int, 0, dashboardTrendWeeks)

	for rows.Next() {
		var weekStart any
		var activeLeads int
		if err := rows.Scan(&weekStart, &activeLeads); err != nil {
			return nil, err
		}
		activeTrend = append(activeTrend, activeLeads)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return padIntTrend(activeTrend), nil
}

func (r *Repository) listQuoteOutcomeTrend(ctx context.Context, organizationID uuid.UUID) ([]int, []int, error) {
	rows, err := r.pool.Query(ctx, `
		WITH weeks AS (
			SELECT generate_series(
				date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
				date_trunc('week', NOW()),
				INTERVAL '1 week'
			) AS week_start
		)
		SELECT
			w.week_start,
			COALESCE(COUNT(*) FILTER (WHERE q.status::text IN ('Accepted', 'Quote_Accepted')), 0) AS accepted_quotes,
			COALESCE(COUNT(*) FILTER (WHERE q.status::text IN ('Sent', 'Quote_Sent')), 0) AS sent_quotes
		FROM weeks w
		LEFT JOIN RAC_quotes q
			ON q.organization_id = $1
			AND q.created_at >= w.week_start
			AND q.created_at < w.week_start + INTERVAL '1 week'
		GROUP BY w.week_start
		ORDER BY w.week_start
	`, organizationID, dashboardTrendWeeks)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	acceptedTrend := make([]int, 0, dashboardTrendWeeks)
	sentTrend := make([]int, 0, dashboardTrendWeeks)

	for rows.Next() {
		var weekStart any
		var accepted int
		var sent int
		if err := rows.Scan(&weekStart, &accepted, &sent); err != nil {
			return nil, nil, err
		}
		acceptedTrend = append(acceptedTrend, accepted)
		sentTrend = append(sentTrend, sent)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return padIntTrend(acceptedTrend), padIntTrend(sentTrend), nil
}

func (r *Repository) listQuotePipelineTrend(ctx context.Context, organizationID uuid.UUID) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		WITH weeks AS (
			SELECT generate_series(
				date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
				date_trunc('week', NOW()),
				INTERVAL '1 week'
			) AS week_start
		)
		SELECT
			w.week_start,
			COALESCE(SUM(CASE WHEN q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted') THEN q.total_cents ELSE 0 END), 0) AS quote_pipeline_cents
		FROM weeks w
		LEFT JOIN RAC_quotes q
			ON q.organization_id = $1
			AND q.created_at >= w.week_start
			AND q.created_at < w.week_start + INTERVAL '1 week'
		GROUP BY w.week_start
		ORDER BY w.week_start
	`, organizationID, dashboardTrendWeeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trend := make([]int64, 0, dashboardTrendWeeks)
	for rows.Next() {
		var weekStart any
		var value int64
		if err := rows.Scan(&weekStart, &value); err != nil {
			return nil, err
		}
		trend = append(trend, value)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return padInt64Trend(trend), nil
}

func (r *Repository) listAvgQuoteValueTrend(ctx context.Context, organizationID uuid.UUID) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		WITH weeks AS (
			SELECT generate_series(
				date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
				date_trunc('week', NOW()),
				INTERVAL '1 week'
			) AS week_start
		)
		SELECT
			w.week_start,
			COALESCE(AVG(CASE WHEN q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted') THEN q.total_cents END)::bigint, 0) AS avg_quote_value_cents
		FROM weeks w
		LEFT JOIN RAC_quotes q
			ON q.organization_id = $1
			AND q.created_at >= w.week_start
			AND q.created_at < w.week_start + INTERVAL '1 week'
		GROUP BY w.week_start
		ORDER BY w.week_start
	`, organizationID, dashboardTrendWeeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trend := make([]int64, 0, dashboardTrendWeeks)
	for rows.Next() {
		var weekStart any
		var value int64
		if err := rows.Scan(&weekStart, &value); err != nil {
			return nil, err
		}
		trend = append(trend, value)
	}

	if err := rows.Err(); err != nil {
		return nil, err
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
