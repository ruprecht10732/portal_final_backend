-- +goose Up
ALTER TABLE RAC_lead_ai_analysis
  ADD COLUMN IF NOT EXISTS resolved_information JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS extracted_facts JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE RAC_lead_ai_analysis
  DROP COLUMN IF EXISTS extracted_facts,
  DROP COLUMN IF EXISTS resolved_information;