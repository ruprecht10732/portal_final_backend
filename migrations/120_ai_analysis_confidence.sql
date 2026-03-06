-- +goose Up
ALTER TABLE RAC_lead_ai_analysis
  ADD COLUMN IF NOT EXISTS composite_confidence DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS confidence_breakdown JSONB,
  ADD COLUMN IF NOT EXISTS risk_flags JSONB;

CREATE INDEX IF NOT EXISTS idx_lead_ai_analysis_confidence
  ON RAC_lead_ai_analysis (organization_id, lead_service_id, composite_confidence);

-- +goose Down
DROP INDEX IF EXISTS idx_lead_ai_analysis_confidence;

ALTER TABLE RAC_lead_ai_analysis
  DROP COLUMN IF EXISTS risk_flags,
  DROP COLUMN IF EXISTS confidence_breakdown,
  DROP COLUMN IF EXISTS composite_confidence;
