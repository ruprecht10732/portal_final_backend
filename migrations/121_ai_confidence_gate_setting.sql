-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS ai_confidence_gate_enabled BOOLEAN NOT NULL DEFAULT false;

UPDATE RAC_organization_settings
SET ai_confidence_gate_enabled = COALESCE(ai_confidence_gate_enabled, false)
WHERE TRUE;

-- +goose Down
ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS ai_confidence_gate_enabled;
