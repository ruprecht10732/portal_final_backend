-- +goose Up

ALTER TABLE RAC_lead_services
    ADD COLUMN IF NOT EXISTS extra_work_amount_cents BIGINT,
    ADD COLUMN IF NOT EXISTS extra_work_notes        TEXT;

-- +goose Down

ALTER TABLE RAC_lead_services
    DROP COLUMN IF EXISTS extra_work_amount_cents,
    DROP COLUMN IF EXISTS extra_work_notes;
