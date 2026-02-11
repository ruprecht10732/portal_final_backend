-- +goose StatementBegin

ALTER TABLE RAC_lead_services
    DROP COLUMN IF EXISTS visit_scheduled_date,
    DROP COLUMN IF EXISTS visit_scout_id,
    DROP COLUMN IF EXISTS visit_measurements,
    DROP COLUMN IF EXISTS visit_access_difficulty,
    DROP COLUMN IF EXISTS visit_notes,
    DROP COLUMN IF EXISTS visit_completed_at;

DROP TABLE IF EXISTS visit_history;

-- +goose StatementEnd

