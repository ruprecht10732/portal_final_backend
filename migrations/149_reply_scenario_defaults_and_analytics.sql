-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS whatsapp_default_reply_scenario TEXT NOT NULL DEFAULT 'generic',
  ADD COLUMN IF NOT EXISTS email_default_reply_scenario TEXT NOT NULL DEFAULT 'generic',
  ADD COLUMN IF NOT EXISTS quote_related_reply_scenario TEXT NOT NULL DEFAULT 'quote_reminder',
  ADD COLUMN IF NOT EXISTS appointment_related_reply_scenario TEXT NOT NULL DEFAULT 'appointment_reminder';

ALTER TABLE RAC_whatsapp_reply_feedback
  ADD COLUMN IF NOT EXISTS scenario TEXT NOT NULL DEFAULT 'generic',
  ADD COLUMN IF NOT EXISTS was_edited BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE RAC_email_reply_feedback
  ADD COLUMN IF NOT EXISTS scenario TEXT NOT NULL DEFAULT 'generic',
  ADD COLUMN IF NOT EXISTS was_edited BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE RAC_whatsapp_reply_feedback
SET was_edited = BTRIM(ai_reply) <> BTRIM(human_reply),
    scenario = COALESCE(NULLIF(BTRIM(scenario), ''), 'generic');

UPDATE RAC_email_reply_feedback
SET was_edited = ai_reply IS NOT NULL AND BTRIM(ai_reply) <> BTRIM(human_reply),
    scenario = COALESCE(NULLIF(BTRIM(scenario), ''), 'generic');

CREATE INDEX IF NOT EXISTS idx_rac_whatsapp_reply_feedback_org_scenario_created
  ON RAC_whatsapp_reply_feedback (organization_id, scenario, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_rac_email_reply_feedback_org_scenario_created
  ON RAC_email_reply_feedback (organization_id, scenario, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_rac_email_reply_feedback_org_scenario_created;
DROP INDEX IF EXISTS idx_rac_whatsapp_reply_feedback_org_scenario_created;

ALTER TABLE RAC_email_reply_feedback
  DROP COLUMN IF EXISTS was_edited,
  DROP COLUMN IF EXISTS scenario;

ALTER TABLE RAC_whatsapp_reply_feedback
  DROP COLUMN IF EXISTS was_edited,
  DROP COLUMN IF EXISTS scenario;

ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS appointment_related_reply_scenario,
  DROP COLUMN IF EXISTS quote_related_reply_scenario,
  DROP COLUMN IF EXISTS email_default_reply_scenario,
  DROP COLUMN IF EXISTS whatsapp_default_reply_scenario;