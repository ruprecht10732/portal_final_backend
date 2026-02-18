-- +goose Up
-- Notes search: lead notes + service consumer notes

CREATE INDEX IF NOT EXISTS idx_lead_notes_fts ON RAC_lead_notes USING GIN (
	(
		setweight(to_tsvector('dutch', rac_immutable_unaccent(coalesce(body, ''))), 'D')
	)
);

CREATE INDEX IF NOT EXISTS idx_lead_services_consumer_note_fts ON RAC_lead_services USING GIN (
	(
		setweight(to_tsvector('dutch', rac_immutable_unaccent(coalesce(consumer_note, ''))), 'D')
	)
) WHERE consumer_note IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_lead_services_consumer_note_fts;
DROP INDEX IF EXISTS idx_lead_notes_fts;
