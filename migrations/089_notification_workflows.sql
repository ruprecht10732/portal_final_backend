-- +goose Up
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

-- +goose Down
DROP TABLE IF EXISTS RAC_notification_workflows;
