-- +goose Up
-- Fix drift: ensure user profile columns and settings table exist.
ALTER TABLE RAC_users
  ADD COLUMN IF NOT EXISTS first_name TEXT,
  ADD COLUMN IF NOT EXISTS last_name TEXT;

CREATE TABLE IF NOT EXISTS RAC_user_settings (
  user_id UUID PRIMARY KEY REFERENCES RAC_users(id) ON DELETE CASCADE,
  preferred_language TEXT NOT NULL DEFAULT 'nl',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO RAC_user_settings (user_id)
SELECT id FROM RAC_users
ON CONFLICT (user_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS RAC_user_settings;
ALTER TABLE RAC_users DROP COLUMN IF EXISTS first_name;
ALTER TABLE RAC_users DROP COLUMN IF EXISTS last_name;
