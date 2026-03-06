-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS ai_council_consensus_mode VARCHAR(50) NOT NULL DEFAULT 'weighted';

UPDATE RAC_organization_settings
SET ai_council_consensus_mode = COALESCE(NULLIF(ai_council_consensus_mode, ''), 'weighted')
WHERE TRUE;

-- +goose Down
ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS ai_council_consensus_mode;
