-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_ai_quote_jobs (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    step TEXT NOT NULL,
    progress_percent INT NOT NULL CHECK (progress_percent >= 0 AND progress_percent <= 100),
    error TEXT,
    quote_id UUID REFERENCES RAC_quotes(id) ON DELETE SET NULL,
    quote_number TEXT,
    item_count INT,
    started_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_user_active
ON RAC_ai_quote_jobs (user_id, updated_at DESC)
WHERE status IN ('pending', 'running');

CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_org_created
ON RAC_ai_quote_jobs (organization_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_quote_jobs_status_finished
ON RAC_ai_quote_jobs (status, finished_at)
WHERE status IN ('completed', 'failed') AND finished_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_ai_quote_jobs_status_finished;
DROP INDEX IF EXISTS idx_ai_quote_jobs_org_created;
DROP INDEX IF EXISTS idx_ai_quote_jobs_user_active;
DROP TABLE IF EXISTS RAC_ai_quote_jobs;