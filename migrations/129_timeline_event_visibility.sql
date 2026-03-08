-- +goose Up
ALTER TABLE lead_timeline_events
ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public',
ADD CONSTRAINT lead_timeline_events_visibility_check
	CHECK (visibility IN ('public', 'internal', 'debug'));

CREATE INDEX idx_timeline_lookup_visibility
	ON lead_timeline_events (lead_id, visibility, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_timeline_lookup_visibility;

ALTER TABLE lead_timeline_events
DROP CONSTRAINT IF EXISTS lead_timeline_events_visibility_check,
DROP COLUMN IF EXISTS visibility;