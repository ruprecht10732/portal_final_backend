-- +goose StatementBegin

ALTER TABLE RAC_appointments
  DROP CONSTRAINT IF EXISTS appointments_status_check;

ALTER TABLE RAC_appointments
  DROP CONSTRAINT IF EXISTS rac_appointments_status_check;

ALTER TABLE RAC_appointments
  ADD CONSTRAINT rac_appointments_status_check
  CHECK (status IN ('scheduled', 'requested', 'completed', 'cancelled', 'no_show'));

-- +goose StatementEnd
