-- +goose Up
ALTER TABLE RAC_lead_services
ADD COLUMN gatekeeper_nurturing_loop_count INTEGER NOT NULL DEFAULT 0,
ADD COLUMN gatekeeper_nurturing_loop_fingerprint TEXT;

CREATE INDEX IF NOT EXISTS idx_lead_services_gatekeeper_nurturing_loop_count
ON RAC_lead_services(gatekeeper_nurturing_loop_count)
WHERE gatekeeper_nurturing_loop_count > 0;

-- +goose Down
DROP INDEX IF EXISTS idx_lead_services_gatekeeper_nurturing_loop_count;

ALTER TABLE RAC_lead_services
DROP COLUMN IF EXISTS gatekeeper_nurturing_loop_fingerprint,
DROP COLUMN IF EXISTS gatekeeper_nurturing_loop_count;