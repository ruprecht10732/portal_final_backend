-- +goose Up
ALTER TABLE RAC_leads
    ADD COLUMN IF NOT EXISTS public_token TEXT UNIQUE,
    ADD COLUMN IF NOT EXISTS public_token_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_leads_public_token ON RAC_leads(public_token) WHERE public_token IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_leads_public_token;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS public_token_expires_at;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS public_token;
