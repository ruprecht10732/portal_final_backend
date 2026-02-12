-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_notification_outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  kind TEXT NOT NULL,
  template TEXT NOT NULL,
  payload JSONB NOT NULL,
  run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_status_run_at
  ON RAC_notification_outbox(status, run_at);
CREATE INDEX IF NOT EXISTS idx_notification_outbox_tenant_id
  ON RAC_notification_outbox(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS RAC_notification_outbox;
