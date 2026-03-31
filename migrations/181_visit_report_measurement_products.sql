-- +goose Up
-- +goose StatementBegin

ALTER TABLE rac_appointment_visit_reports
    ADD COLUMN IF NOT EXISTS measurement_products JSONB;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE rac_appointment_visit_reports
    DROP COLUMN IF EXISTS measurement_products;

-- +goose StatementEnd
