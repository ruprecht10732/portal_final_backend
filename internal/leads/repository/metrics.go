package repository

import (
	"context"

	"github.com/google/uuid"
)

// LeadMetrics aggregates KPI values for the dashboard.
type LeadMetrics struct {
	TotalLeads          int
	DisqualifiedLeads   int
	ProjectedValueCents int64
	Touchpoints         int
}

// GetMetrics returns KPI aggregates for active (non-deleted) RAC_leads within an organization.
func (r *Repository) GetMetrics(ctx context.Context, organizationID uuid.UUID) (LeadMetrics, error) {
	var metrics LeadMetrics
	err := r.pool.QueryRow(ctx, `
		SELECT
			(
				SELECT COUNT(*)
				FROM RAC_leads
				WHERE organization_id = $1 AND deleted_at IS NULL
			) AS total_leads,
			(
				SELECT COUNT(DISTINCT l.id)
				FROM RAC_leads l
				LEFT JOIN RAC_lead_services ls ON ls.lead_id = l.id
				WHERE l.organization_id = $1 AND l.deleted_at IS NULL
					AND ls.status = 'Disqualified'
			) AS disqualified_leads,
			(
				SELECT COALESCE(SUM(projected_value_cents), 0)
				FROM RAC_leads
				WHERE organization_id = $1 AND deleted_at IS NULL
			) AS projected_value_cents,
			COALESCE((
				SELECT COUNT(*)
				FROM RAC_lead_activity la
				JOIN RAC_leads l ON l.id = la.lead_id
				WHERE l.organization_id = $1 AND l.deleted_at IS NULL
			), 0) AS touchpoints
		
	`, organizationID).Scan(
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
