-- +goose Up
-- 051_quote_preview_tokens.sql
-- Read-only preview tokens for internal agent preview links

ALTER TABLE RAC_quotes
ADD COLUMN preview_token TEXT UNIQUE,
ADD COLUMN preview_token_expires_at TIMESTAMPTZ;

CREATE INDEX idx_quotes_preview_token ON RAC_quotes(preview_token) WHERE preview_token IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_quotes_preview_token;
ALTER TABLE RAC_quotes DROP COLUMN IF EXISTS preview_token_expires_at;
ALTER TABLE RAC_quotes DROP COLUMN IF EXISTS preview_token;
