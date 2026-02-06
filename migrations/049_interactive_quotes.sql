-- 049_interactive_quotes.sql
-- Interactive Quote Proposal System: public access, customer selection, annotations

-- 1. Security & Access columns on quotes
ALTER TABLE RAC_quotes
ADD COLUMN public_token TEXT UNIQUE,
ADD COLUMN public_token_expires_at TIMESTAMPTZ,
ADD COLUMN viewed_at TIMESTAMPTZ,
ADD COLUMN accepted_at TIMESTAMPTZ,
ADD COLUMN rejected_at TIMESTAMPTZ,
ADD COLUMN rejection_reason TEXT,
ADD COLUMN signature_name TEXT,
ADD COLUMN signature_data TEXT,
ADD COLUMN signature_ip TEXT,
ADD COLUMN pdf_file_key TEXT;

-- Partial unique index for non-null tokens (the UNIQUE above handles it, but explicit)
CREATE INDEX idx_quotes_public_token ON RAC_quotes(public_token) WHERE public_token IS NOT NULL;

-- 2. Customer selection state on line items
ALTER TABLE RAC_quote_items
ADD COLUMN is_selected BOOLEAN NOT NULL DEFAULT true;

-- 3. Quote annotations (questions/comments per line item)
CREATE TABLE RAC_quote_annotations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_item_id UUID NOT NULL REFERENCES RAC_quote_items(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    author_type TEXT NOT NULL CHECK (author_type IN ('customer', 'agent')),
    author_id UUID,
    text TEXT NOT NULL,
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quote_annotations_item ON RAC_quote_annotations(quote_item_id);
