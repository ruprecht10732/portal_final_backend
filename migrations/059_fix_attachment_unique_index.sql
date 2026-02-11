-- +goose Up
-- 055_fix_attachment_unique_index.sql
-- Change dedup key from (quote_id, filename) to (quote_id, file_key).
-- Multiple catalog products can share the same filename (e.g. "productblad.pdf").

DROP INDEX IF EXISTS idx_quote_attachments_unique_filename;

CREATE UNIQUE INDEX idx_quote_attachments_unique_file_key
    ON RAC_quote_attachments (quote_id, file_key);
