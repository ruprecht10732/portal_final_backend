-- +goose Up
-- Add advanced AI orchestration toggles and long-lived decision memory.

ALTER TABLE RAC_organization_settings
    ADD COLUMN IF NOT EXISTS ai_adaptive_reasoning_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS ai_experience_memory_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS ai_council_enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS RAC_ai_decision_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_id UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    service_type VARCHAR(100) NOT NULL,
    decision_type VARCHAR(50) NOT NULL,
    outcome VARCHAR(50) NOT NULL,
    confidence NUMERIC(4,3),
    context_summary TEXT NOT NULL,
    action_summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_decision_memory_org_service_created
    ON RAC_ai_decision_memory (organization_id, service_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_decision_memory_org_type_created
    ON RAC_ai_decision_memory (organization_id, decision_type, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_decision_memory_org_type_created;
DROP INDEX IF EXISTS idx_ai_decision_memory_org_service_created;
DROP TABLE IF EXISTS RAC_ai_decision_memory;

ALTER TABLE RAC_organization_settings
    DROP COLUMN IF EXISTS ai_council_enabled,
    DROP COLUMN IF EXISTS ai_experience_memory_enabled,
    DROP COLUMN IF EXISTS ai_adaptive_reasoning_enabled;
