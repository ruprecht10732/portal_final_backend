package repository

import "context"

// LeadMetrics aggregates KPI values for the dashboard.
type LeadMetrics struct {
	TotalLeads          int
	DisqualifiedLeads   int
	ProjectedValueCents int64
	Touchpoints         int
}

// GetMetrics returns KPI aggregates for active (non-deleted) leads.
func (r *Repository) GetMetrics(ctx context.Context) (LeadMetrics, error) {
	var metrics LeadMetrics
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE deleted_at IS NULL) AS total_leads,
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND status = 'Bad_Lead') AS disqualified_leads,
			COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN projected_value_cents ELSE 0 END), 0) AS projected_value_cents,
			COALESCE((
				SELECT COUNT(*)
				FROM lead_activity la
				JOIN leads l ON l.id = la.lead_id
				WHERE l.deleted_at IS NULL
			), 0) AS touchpoints
		FROM leads
	`).Scan(
		&metrics.TotalLeads,
		&metrics.DisqualifiedLeads,
		&metrics.ProjectedValueCents,
		&metrics.Touchpoints,
	)
	if err != nil {
		return LeadMetrics{}, err
	}
	return metrics, nil
}
