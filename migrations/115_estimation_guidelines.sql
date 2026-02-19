-- +goose Up
-- Add estimation_guidelines to RAC_service_types for AI estimator material scope rules
ALTER TABLE RAC_service_types
ADD COLUMN IF NOT EXISTS estimation_guidelines TEXT;

-- +goose Down
ALTER TABLE RAC_service_types
DROP COLUMN IF EXISTS estimation_guidelines;
