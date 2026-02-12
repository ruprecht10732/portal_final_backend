-- +goose Up
-- Foundation schema for workflow engine (backend-first).
-- Backward compatibility note:
-- Existing RAC_notification_workflows remains untouched and continues to support current runtime.
-- New tables are additive and will be consumed by the new workflow engine incrementally.

CREATE TABLE IF NOT EXISTS RAC_workflows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  workflow_key TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  quote_valid_days_override INT CHECK (quote_valid_days_override IS NULL OR quote_valid_days_override BETWEEN 1 AND 365),
  quote_payment_days_override INT CHECK (quote_payment_days_override IS NULL OR quote_payment_days_override BETWEEN 1 AND 365),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, workflow_key)
);

CREATE INDEX IF NOT EXISTS idx_workflows_org_enabled
  ON RAC_workflows(organization_id, enabled);

CREATE TABLE IF NOT EXISTS RAC_workflow_steps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  workflow_id UUID NOT NULL REFERENCES RAC_workflows(id) ON DELETE CASCADE,
  trigger TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('whatsapp', 'email')),
  audience TEXT NOT NULL DEFAULT 'lead',
  action TEXT NOT NULL DEFAULT 'send_message',
  step_order INT NOT NULL DEFAULT 1 CHECK (step_order > 0),
  delay_minutes INT NOT NULL DEFAULT 0 CHECK (delay_minutes >= 0),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  recipient_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  template_subject TEXT,
  template_body TEXT,
  stop_on_reply BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (workflow_id, trigger, channel, step_order)
);

CREATE INDEX IF NOT EXISTS idx_workflow_steps_org_trigger
  ON RAC_workflow_steps(organization_id, trigger, channel, enabled);

CREATE TABLE IF NOT EXISTS RAC_workflow_assignment_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  workflow_id UUID NOT NULL REFERENCES RAC_workflows(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  priority INT NOT NULL DEFAULT 100,
  lead_source TEXT,
  lead_service_type TEXT,
  pipeline_stage TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workflow_assignment_rules_org_priority
  ON RAC_workflow_assignment_rules(organization_id, enabled, priority ASC, created_at ASC);

CREATE TABLE IF NOT EXISTS RAC_lead_workflow_overrides (
  lead_id UUID PRIMARY KEY REFERENCES RAC_leads(id) ON DELETE CASCADE,
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  workflow_id UUID REFERENCES RAC_workflows(id) ON DELETE SET NULL,
  override_mode TEXT NOT NULL DEFAULT 'manual' CHECK (override_mode IN ('manual', 'manual_lock', 'clear')),
  reason TEXT,
  assigned_by UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lead_workflow_overrides_org_workflow
  ON RAC_lead_workflow_overrides(organization_id, workflow_id);

-- Compatibility helper view so old one-row workflow configs can be inspected
-- next to step-based workflow records during migration/rollout.
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

-- +goose Down
DROP VIEW IF EXISTS RAC_workflow_legacy_notification_rules;

DROP INDEX IF EXISTS idx_lead_workflow_overrides_org_workflow;
DROP TABLE IF EXISTS RAC_lead_workflow_overrides;

DROP INDEX IF EXISTS idx_workflow_assignment_rules_org_priority;
DROP TABLE IF EXISTS RAC_workflow_assignment_rules;

DROP INDEX IF EXISTS idx_workflow_steps_org_trigger;
DROP TABLE IF EXISTS RAC_workflow_steps;

DROP INDEX IF EXISTS idx_workflows_org_enabled;
DROP TABLE IF EXISTS RAC_workflows;
