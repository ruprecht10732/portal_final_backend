-- +goose Up
ALTER TABLE RAC_catalog_search_log
  ADD COLUMN IF NOT EXISTS run_id TEXT,
  ADD COLUMN IF NOT EXISTS tool_name TEXT,
  ADD COLUMN IF NOT EXISTS agent_name TEXT;

CREATE INDEX IF NOT EXISTS idx_catalog_search_log_org_run_created
  ON RAC_catalog_search_log (organization_id, run_id, created_at DESC)
  WHERE run_id IS NOT NULL;

ALTER TABLE RAC_quote_pricing_outcomes
  ADD COLUMN IF NOT EXISTS estimator_run_id TEXT;

UPDATE RAC_quote_pricing_outcomes o
SET estimator_run_id = s.estimator_run_id
FROM RAC_quote_pricing_snapshots s
WHERE o.snapshot_id = s.id
  AND o.estimator_run_id IS NULL
  AND s.estimator_run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_quote_pricing_outcomes_org_run_created
  ON RAC_quote_pricing_outcomes (organization_id, estimator_run_id, created_at DESC)
  WHERE estimator_run_id IS NOT NULL;

ALTER TABLE RAC_quote_pricing_corrections
  ADD COLUMN IF NOT EXISTS estimator_run_id TEXT;

UPDATE RAC_quote_pricing_corrections c
SET estimator_run_id = s.estimator_run_id
FROM RAC_quote_pricing_snapshots s
WHERE c.snapshot_id = s.id
  AND c.estimator_run_id IS NULL
  AND s.estimator_run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_quote_pricing_corrections_org_run_created
  ON RAC_quote_pricing_corrections (organization_id, estimator_run_id, created_at DESC)
  WHERE estimator_run_id IS NOT NULL;

ALTER TABLE RAC_ai_quote_jobs
  ADD COLUMN IF NOT EXISTS feedback_rating INT,
  ADD COLUMN IF NOT EXISTS feedback_comment TEXT,
  ADD COLUMN IF NOT EXISTS feedback_submitted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS cancellation_reason TEXT,
  ADD COLUMN IF NOT EXISTS viewed_at TIMESTAMPTZ;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS ai_quote_jobs_feedback_rating_check;

ALTER TABLE RAC_ai_quote_jobs
  ADD CONSTRAINT ai_quote_jobs_feedback_rating_check
  CHECK (feedback_rating IS NULL OR feedback_rating IN (-1, 1));

-- +goose Down
ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS ai_quote_jobs_feedback_rating_check;

ALTER TABLE RAC_ai_quote_jobs
  DROP COLUMN IF EXISTS viewed_at,
  DROP COLUMN IF EXISTS cancellation_reason,
  DROP COLUMN IF EXISTS feedback_submitted_at,
  DROP COLUMN IF EXISTS feedback_comment,
  DROP COLUMN IF EXISTS feedback_rating;

DROP INDEX IF EXISTS idx_quote_pricing_corrections_org_run_created;

ALTER TABLE RAC_quote_pricing_corrections
  DROP COLUMN IF EXISTS estimator_run_id;

DROP INDEX IF EXISTS idx_quote_pricing_outcomes_org_run_created;

ALTER TABLE RAC_quote_pricing_outcomes
  DROP COLUMN IF EXISTS estimator_run_id;

DROP INDEX IF EXISTS idx_catalog_search_log_org_run_created;

ALTER TABLE RAC_catalog_search_log
  DROP COLUMN IF EXISTS agent_name,
  DROP COLUMN IF EXISTS tool_name,
  DROP COLUMN IF EXISTS run_id;