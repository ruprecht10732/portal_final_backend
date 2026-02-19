-- +goose Up
-- Track product catalog search queries and misses to improve catalog coverage.
CREATE TABLE IF NOT EXISTS RAC_catalog_search_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    query TEXT NOT NULL,
    collection TEXT NOT NULL,
    result_count INT NOT NULL DEFAULT 0,
    top_score DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_catalog_search_log_org_created
    ON RAC_catalog_search_log (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_catalog_search_log_service_created
    ON RAC_catalog_search_log (lead_service_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS RAC_catalog_search_log;
