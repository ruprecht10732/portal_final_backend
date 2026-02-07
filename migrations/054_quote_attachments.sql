-- 054_quote_attachments.sql
-- Adds attachment (PDF) and URL (voorwaarden) tracking to quotes.

-- Source enum for attachment origin
CREATE TYPE rac_quote_attachment_source AS ENUM ('catalog', 'manual');

-- Quote attachments (PDF documents)
CREATE TABLE IF NOT EXISTS RAC_quote_attachments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id            UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL,
    filename            TEXT NOT NULL,
    file_key            TEXT NOT NULL,
    source              rac_quote_attachment_source NOT NULL DEFAULT 'catalog',
    catalog_product_id  UUID,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order          INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Deduplication: same filename cannot appear twice on the same quote
CREATE UNIQUE INDEX idx_quote_attachments_unique_filename
    ON RAC_quote_attachments (quote_id, filename);

CREATE INDEX idx_quote_attachments_quote_id
    ON RAC_quote_attachments (quote_id);

-- Quote URLs (voorwaarden / terms links)
CREATE TABLE IF NOT EXISTS RAC_quote_urls (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id            UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL,
    label               TEXT NOT NULL,
    href                TEXT NOT NULL,
    accepted            BOOLEAN NOT NULL DEFAULT FALSE,
    catalog_product_id  UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quote_urls_quote_id
    ON RAC_quote_urls (quote_id);
