-- +goose Up
-- Fix drift: ensure onboarding_completed_at exists on RAC_users.
ALTER TABLE RAC_users ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE RAC_users DROP COLUMN IF EXISTS onboarding_completed_at;
