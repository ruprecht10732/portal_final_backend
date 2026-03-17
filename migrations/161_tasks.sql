-- +goose Up
ALTER TABLE RAC_users
  ADD COLUMN IF NOT EXISTS phone TEXT;

CREATE TABLE IF NOT EXISTS RAC_tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  scope_type TEXT NOT NULL CHECK (scope_type IN ('global', 'lead_service')),
  lead_id UUID REFERENCES RAC_leads(id) ON DELETE CASCADE,
  lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
  assigned_user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE RESTRICT,
  created_by_user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE RESTRICT,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'completed', 'cancelled')),
  priority TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
  due_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT rac_tasks_scope_check CHECK (
    (scope_type = 'global' AND lead_id IS NULL AND lead_service_id IS NULL)
    OR
    (scope_type = 'lead_service' AND lead_id IS NOT NULL AND lead_service_id IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS idx_rac_tasks_tenant_status_due
  ON RAC_tasks (tenant_id, status, due_at NULLS LAST, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rac_tasks_assigned_user
  ON RAC_tasks (assigned_user_id, status, due_at NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_rac_tasks_lead_service
  ON RAC_tasks (lead_service_id, status, created_at DESC)
  WHERE lead_service_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS RAC_task_reminders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id UUID NOT NULL UNIQUE REFERENCES RAC_tasks(id) ON DELETE CASCADE,
  tenant_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  enabled BOOLEAN NOT NULL DEFAULT true,
  send_email BOOLEAN NOT NULL DEFAULT true,
  send_whatsapp BOOLEAN NOT NULL DEFAULT false,
  next_run_at TIMESTAMPTZ,
  repeat_daily BOOLEAN NOT NULL DEFAULT false,
  last_sent_at TIMESTAMPTZ,
  last_triggered_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rac_task_reminders_due
  ON RAC_task_reminders (tenant_id, enabled, next_run_at)
  WHERE enabled = true AND next_run_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_rac_task_reminders_due;
DROP TABLE IF EXISTS RAC_task_reminders;
DROP INDEX IF EXISTS idx_rac_tasks_lead_service;
DROP INDEX IF EXISTS idx_rac_tasks_assigned_user;
DROP INDEX IF EXISTS idx_rac_tasks_tenant_status_due;
DROP TABLE IF EXISTS RAC_tasks;
ALTER TABLE RAC_users DROP COLUMN IF EXISTS phone;