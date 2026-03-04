-- +goose Up
ALTER TABLE RAC_notification_outbox
  ADD COLUMN IF NOT EXISTS lead_id UUID,
  ADD COLUMN IF NOT EXISTS service_id UUID;

CREATE INDEX IF NOT EXISTS idx_notification_outbox_lead_id
  ON RAC_notification_outbox(lead_id);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_lead_run_at
  ON RAC_notification_outbox(tenant_id, lead_id, status, run_at);

-- +goose Down
DROP INDEX IF EXISTS idx_notification_outbox_lead_run_at;
DROP INDEX IF EXISTS idx_notification_outbox_lead_id;

ALTER TABLE RAC_notification_outbox
  DROP COLUMN IF EXISTS service_id,
  DROP COLUMN IF EXISTS lead_id;
