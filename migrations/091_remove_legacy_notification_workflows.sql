-- +goose Up
-- Strict workflow-engine cutover:
-- remove legacy notification workflow storage and compatibility view.

DROP VIEW IF EXISTS RAC_workflow_legacy_notification_rules;
DROP INDEX IF EXISTS idx_notification_workflows_org;
DROP TABLE IF EXISTS RAC_notification_workflows;

-- +goose Down
CREATE TABLE IF NOT EXISTS RAC_notification_workflows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  trigger TEXT NOT NULL,
  channel TEXT NOT NULL DEFAULT 'whatsapp',
  audience TEXT NOT NULL DEFAULT 'lead',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  delay_minutes INT NOT NULL DEFAULT 0,
  lead_source TEXT,
  template_text TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, trigger, channel, audience)
);

CREATE INDEX IF NOT EXISTS idx_notification_workflows_org
  ON RAC_notification_workflows(organization_id);

CREATE OR REPLACE VIEW RAC_workflow_legacy_notification_rules AS
SELECT
  id,
  organization_id,
  trigger,
  channel,
  audience,
  enabled,
  delay_minutes,
  lead_source,
  template_text,
  created_at,
  updated_at
FROM RAC_notification_workflows;
