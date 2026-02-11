-- +goose Up
-- Add Quote_Sent stage to the pipeline_stage enum
-- This handles the holding state between Estimation and Partner Dispatch
ALTER TYPE pipeline_stage ADD VALUE IF NOT EXISTS 'Quote_Sent' AFTER 'Ready_For_Estimator';

-- +goose Down
SELECT 1;