-- +goose Up
-- Add explicit is_relevant flag to photo analyses so the LLM can
-- declare relevance structurally instead of relying on brittle substring matching.

ALTER TABLE RAC_lead_photo_analyses
    ADD COLUMN IF NOT EXISTS is_relevant BOOLEAN;

COMMENT ON COLUMN RAC_lead_photo_analyses.is_relevant IS
    'Explicit boolean set by the LLM indicating whether the photos are relevant to the service type';

-- +goose Down
ALTER TABLE RAC_lead_photo_analyses DROP COLUMN IF EXISTS is_relevant;
