-- +goose Up
ALTER TABLE RAC_lead_services
ADD COLUMN agent_cycle_count INTEGER NOT NULL DEFAULT 0,
ADD COLUMN agent_cycle_fingerprint TEXT,
ADD COLUMN agent_cycle_last_transition TEXT;

CREATE INDEX IF NOT EXISTS idx_lead_services_agent_cycle_count
ON RAC_lead_services(agent_cycle_count)
WHERE agent_cycle_count > 0;

-- +goose Down
DROP INDEX IF EXISTS idx_lead_services_agent_cycle_count;

ALTER TABLE RAC_lead_services
DROP COLUMN IF EXISTS agent_cycle_last_transition,
DROP COLUMN IF EXISTS agent_cycle_fingerprint,
DROP COLUMN IF EXISTS agent_cycle_count;
