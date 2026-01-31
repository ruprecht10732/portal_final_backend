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
			(
				SELECT COUNT(*)
				FROM leads
				WHERE deleted_at IS NULL
			) AS total_leads,
			(
				SELECT COUNT(DISTINCT l.id)
				FROM leads l
				LEFT JOIN lead_services ls ON ls.lead_id = l.id
				WHERE l.deleted_at IS NULL
					AND ls.status = 'Bad_Lead'
			) AS disqualified_leads,
			(
				SELECT COALESCE(SUM(projected_value_cents), 0)
				FROM leads
				WHERE deleted_at IS NULL
			) AS projected_value_cents,
			COALESCE((
				SELECT COUNT(*)
				FROM lead_activity la
				JOIN leads l ON l.id = la.lead_id
				WHERE l.deleted_at IS NULL
			), 0) AS touchpoints
		
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
