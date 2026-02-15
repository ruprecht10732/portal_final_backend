-- +goose Up
-- Add Quote_Draft stage to represent drafted (not yet sent) quotes.
ALTER TYPE pipeline_stage ADD VALUE IF NOT EXISTS 'Quote_Draft' AFTER 'Ready_For_Estimator';

-- +goose Down
SELECT 1;
