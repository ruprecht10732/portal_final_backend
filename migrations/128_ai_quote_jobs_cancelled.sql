-- +goose Up
ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS rac_ai_quote_jobs_status_check;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS rac_ai_quote_jobs_status_chk;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS RAC_ai_quote_jobs_status_check;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS catalog_ai_quote_jobs_status_check;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS RAC_ai_quote_jobs_status_chk;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS ai_quote_jobs_status_check;

ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS ai_quote_jobs_status_chk;

ALTER TABLE RAC_ai_quote_jobs
  ADD CONSTRAINT ai_quote_jobs_status_check
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));

DROP INDEX IF EXISTS idx_ai_quote_jobs_user_active;
CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_user_active
ON RAC_ai_quote_jobs (user_id, updated_at DESC)
WHERE status IN ('pending', 'running');

DROP INDEX IF EXISTS idx_ai_quote_jobs_status_finished;
CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_status_finished
ON RAC_ai_quote_jobs (status, finished_at)
WHERE status IN ('completed', 'failed', 'cancelled') AND finished_at IS NOT NULL;

-- +goose Down
ALTER TABLE RAC_ai_quote_jobs
  DROP CONSTRAINT IF EXISTS ai_quote_jobs_status_check;

ALTER TABLE RAC_ai_quote_jobs
  ADD CONSTRAINT ai_quote_jobs_status_check
  CHECK (status IN ('pending', 'running', 'completed', 'failed'));

DROP INDEX IF EXISTS idx_ai_quote_jobs_user_active;
CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_user_active
ON RAC_ai_quote_jobs (user_id, updated_at DESC)
WHERE status IN ('pending', 'running');

DROP INDEX IF EXISTS idx_ai_quote_jobs_status_finished;
CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_status_finished
ON RAC_ai_quote_jobs (status, finished_at)
WHERE status IN ('completed', 'failed') AND finished_at IS NOT NULL;