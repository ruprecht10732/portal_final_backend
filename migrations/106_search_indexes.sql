-- +goose Up
-- Enable unaccent for accent-insensitive search (e.g. "café" matches "cafe")
CREATE EXTENSION IF NOT EXISTS unaccent SCHEMA public;

-- unaccent(text) is STABLE, which cannot be used in index expressions.
-- We provide an IMMUTABLE wrapper to make expression indexes possible.
-- This is safe as long as the unaccent dictionaries are not changed at runtime.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rac_immutable_unaccent(text)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
	SELECT public.unaccent($1)
$$;
-- +goose StatementEnd

-- Leads: Name, Email, Phone, City (weighted)
CREATE INDEX IF NOT EXISTS idx_leads_fts ON RAC_leads USING GIN (
	(
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(consumer_first_name, ''))), 'A') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(consumer_last_name, ''))), 'A') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(consumer_email, ''))), 'B') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(consumer_phone, ''))), 'B') ||
		setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(address_city, ''))), 'C')
	)
);

-- Quotes: Number, Notes (weighted)
CREATE INDEX IF NOT EXISTS idx_quotes_fts ON RAC_quotes USING GIN (
	(
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(quote_number, ''))), 'A') ||
		setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(notes, ''))), 'D')
	)
);

-- Partners: Business Name, Contact (weighted)
CREATE INDEX IF NOT EXISTS idx_partners_fts ON RAC_partners USING GIN (
	(
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(business_name, ''))), 'A') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(contact_name, ''))), 'B') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(contact_email, ''))), 'C')
	)
);

-- Appointments: Title, Description, Location (weighted)
CREATE INDEX IF NOT EXISTS idx_appointments_fts ON RAC_appointments USING GIN (
	(
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(title, ''))), 'A') ||
		setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(description, ''))), 'D') ||
		setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(location, ''))), 'C')
	)
);

-- +goose Down
DROP INDEX IF EXISTS idx_appointments_fts;
DROP INDEX IF EXISTS idx_partners_fts;
DROP INDEX IF EXISTS idx_quotes_fts;
DROP INDEX IF EXISTS idx_leads_fts;

DROP FUNCTION IF EXISTS rac_immutable_unaccent(text);

-- Note: we intentionally do not drop the `unaccent` extension here.
-- It may be used by other modules, and dropping it in a down migration could break unrelated features.
