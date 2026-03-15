-- +goose Up

CREATE INDEX IF NOT EXISTS idx_quotes_org_lead_created_at
    ON RAC_quotes (organization_id, lead_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_quotes_org_service_created_at
    ON RAC_quotes (organization_id, lead_service_id, created_at DESC)
    WHERE lead_service_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_quotes_org_service_created_at;
DROP INDEX IF EXISTS idx_quotes_org_lead_created_at;
