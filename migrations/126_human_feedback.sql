-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_human_feedback (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
  lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
  field_changed VARCHAR(100) NOT NULL,
  ai_value JSONB NOT NULL,
  human_value JSONB NOT NULL,
  delta_percentage DOUBLE PRECISION,
  context_embedding_id TEXT,
  applied_to_memory BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_human_feedback_org_created
  ON RAC_human_feedback (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_human_feedback_org_service
  ON RAC_human_feedback (organization_id, lead_service_id);

-- +goose Down
DROP INDEX IF EXISTS idx_human_feedback_org_service;
DROP INDEX IF EXISTS idx_human_feedback_org_created;
DROP TABLE IF EXISTS RAC_human_feedback;
