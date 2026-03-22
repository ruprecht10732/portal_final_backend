package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/logger"
)

// AIActivitySummary aggregates AI work done in the last 24 hours.
type AIActivitySummary struct {
	GatekeeperRuns  int
	EstimatorRuns   int
	DispatcherRuns  int
	QuotesGenerated int
	PhotosAnalyzed  int
	OffersProcessed int
}

// PipelineSnapshot counts services per pipeline stage.
type PipelineSnapshot struct {
	Triage             int
	Nurturing          int
	Estimation         int
	Proposal           int
	Fulfillment        int
	ManualIntervention int
}

// DigestData holds all data for the daily morning digest email.
type DigestData struct {
	OrganizationName string
	Date             string
	AIActivity       AIActivitySummary
	StaleLeads       []StaleLeadItem
	StaleLeadCount   int
	PipelineSnapshot PipelineSnapshot
	DashboardURL     string
}

// DailyDigestService aggregates data for the morning digest.
type DailyDigestService struct {
	pool     *pgxpool.Pool
	detector *StaleLeadDetector
	log      *logger.Logger
}

// NewDailyDigestService creates a new DailyDigestService.
func NewDailyDigestService(pool *pgxpool.Pool, detector *StaleLeadDetector, log *logger.Logger) *DailyDigestService {
	return &DailyDigestService{pool: pool, detector: detector, log: log}
}

const aiActivityQuery = `
SELECT
	COALESCE(SUM(CASE WHEN actor_name = 'Gatekeeper' THEN 1 ELSE 0 END), 0) AS gatekeeper_runs,
	COALESCE(SUM(CASE WHEN actor_name = 'Estimator' THEN 1 ELSE 0 END), 0) AS estimator_runs,
	COALESCE(SUM(CASE WHEN actor_name = 'Dispatcher' THEN 1 ELSE 0 END), 0) AS dispatcher_runs,
	COALESCE(SUM(CASE WHEN event_type = 'quote_generated' THEN 1 ELSE 0 END), 0) AS quotes_generated,
	COALESCE(SUM(CASE WHEN event_type = 'photo_analysis_completed' THEN 1 ELSE 0 END), 0) AS photos_analyzed,
	COALESCE(SUM(CASE WHEN event_type = 'partner_offer' THEN 1 ELSE 0 END), 0) AS offers_processed
FROM lead_timeline_events
WHERE organization_id = $1
	AND actor_type = 'AI'
	AND created_at >= NOW() - INTERVAL '24 hours'
`

const pipelineSnapshotQuery = `
SELECT
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Triage' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Nurturing' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Estimation' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Proposal' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Fulfillment' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN pipeline_stage = 'Manual_Intervention' THEN 1 ELSE 0 END), 0)
FROM RAC_lead_services
WHERE organization_id = $1
	AND pipeline_stage NOT IN ('Completed', 'Lost')
`

// GenerateDigest collects all digest data for one organization.
func (s *DailyDigestService) GenerateDigest(ctx context.Context, organizationID uuid.UUID, organizationName, dashboardURL string) (*DigestData, error) {
	loc, _ := time.LoadLocation("Europe/Amsterdam")
	now := time.Now().In(loc)

	digest := &DigestData{
		OrganizationName: organizationName,
		Date:             now.Format("02 januari 2006"),
		DashboardURL:     dashboardURL,
	}

	// AI Activity
	err := s.pool.QueryRow(ctx, aiActivityQuery, organizationID).Scan(
		&digest.AIActivity.GatekeeperRuns,
		&digest.AIActivity.EstimatorRuns,
		&digest.AIActivity.DispatcherRuns,
		&digest.AIActivity.QuotesGenerated,
		&digest.AIActivity.PhotosAnalyzed,
		&digest.AIActivity.OffersProcessed,
	)
	if err != nil {
		return nil, fmt.Errorf("ai activity query: %w", err)
	}

	// Pipeline snapshot
	err = s.pool.QueryRow(ctx, pipelineSnapshotQuery, organizationID).Scan(
		&digest.PipelineSnapshot.Triage,
		&digest.PipelineSnapshot.Nurturing,
		&digest.PipelineSnapshot.Estimation,
		&digest.PipelineSnapshot.Proposal,
		&digest.PipelineSnapshot.Fulfillment,
		&digest.PipelineSnapshot.ManualIntervention,
	)
	if err != nil {
		return nil, fmt.Errorf("pipeline snapshot query: %w", err)
	}

	// Stale leads (cap at 10 for digest email)
	staleLeads, err := s.detector.ListStaleLeadServices(ctx, organizationID, 10)
	if err != nil {
		s.log.Warn("digest: stale leads query failed, continuing without", "orgId", organizationID, "error", err)
		staleLeads = nil
	}
	digest.StaleLeads = staleLeads
	digest.StaleLeadCount = len(staleLeads)

	return digest, nil
}
