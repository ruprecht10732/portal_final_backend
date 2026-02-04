-- +goose Up
-- +goose StatementBegin

ALTER TABLE appointments
  ADD COLUMN IF NOT EXISTS meeting_link TEXT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE appointments
  DROP COLUMN IF EXISTS meeting_link;

-- +goose StatementEnd
