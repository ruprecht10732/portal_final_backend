-- +goose StatementBegin

ALTER TABLE appointments
  ADD COLUMN IF NOT EXISTS meeting_link TEXT;

-- +goose StatementEnd

