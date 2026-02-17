-- +goose Up
ALTER TABLE RAC_provider_integrations
  DROP CONSTRAINT IF EXISTS rac_provider_integrations_connected_by_fkey;

ALTER TABLE RAC_provider_integrations
  ADD CONSTRAINT rac_provider_integrations_connected_by_fkey
  FOREIGN KEY (connected_by) REFERENCES RAC_users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE RAC_provider_integrations
  DROP CONSTRAINT IF EXISTS rac_provider_integrations_connected_by_fkey;

ALTER TABLE RAC_provider_integrations
  ADD CONSTRAINT rac_provider_integrations_connected_by_fkey
  FOREIGN KEY (connected_by) REFERENCES RAC_users(id);
