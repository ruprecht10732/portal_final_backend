-- +goose Up
-- Extend organization settings with AI/agent configuration.
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS ai_auto_disqualify_junk  BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS ai_auto_dispatch         BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS ai_auto_estimate         BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS catalog_gap_threshold    INT NOT NULL DEFAULT 3,
  ADD COLUMN IF NOT EXISTS catalog_gap_lookback_days INT NOT NULL DEFAULT 30;

-- Backfill existing rows (defaults apply, but keep this explicit for clarity/older engines).
UPDATE RAC_organization_settings
SET
  ai_auto_disqualify_junk = COALESCE(ai_auto_disqualify_junk, true),
  ai_auto_dispatch = COALESCE(ai_auto_dispatch, false),
  ai_auto_estimate = COALESCE(ai_auto_estimate, true),
  catalog_gap_threshold = COALESCE(catalog_gap_threshold, 3),
  catalog_gap_lookback_days = COALESCE(catalog_gap_lookback_days, 30)
WHERE TRUE;

-- +goose Down
ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS ai_auto_disqualify_junk,
  DROP COLUMN IF EXISTS ai_auto_dispatch,
  DROP COLUMN IF EXISTS ai_auto_estimate,
  DROP COLUMN IF EXISTS catalog_gap_threshold,
  DROP COLUMN IF EXISTS catalog_gap_lookback_days;
