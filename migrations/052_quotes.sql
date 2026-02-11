-- +goose Up
-- 048_quotes.sql
-- Quotes (Offertes) feature: header + line items tables

CREATE TYPE quote_status AS ENUM ('Draft', 'Sent', 'Accepted', 'Rejected', 'Expired');

CREATE TABLE RAC_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,

    quote_number TEXT NOT NULL,
    status quote_status NOT NULL DEFAULT 'Draft',
    valid_until TIMESTAMPTZ,
    notes TEXT,
    pricing_mode TEXT NOT NULL DEFAULT 'exclusive', -- 'exclusive' or 'inclusive'

    -- Discount
    discount_type TEXT NOT NULL DEFAULT 'percentage', -- 'percentage' or 'fixed'
    discount_value BIGINT NOT NULL DEFAULT 0,         -- percentage (bps) or fixed (cents)

    -- Calculation Snapshots (stored for performance and history)
    subtotal_cents BIGINT NOT NULL DEFAULT 0,
    discount_amount_cents BIGINT NOT NULL DEFAULT 0,
    tax_total_cents BIGINT NOT NULL DEFAULT 0,
    total_cents BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE RAC_quote_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,

    description TEXT NOT NULL,
    quantity TEXT NOT NULL DEFAULT '1 x',           -- Free-form: "5 x", "10 m²", "3 uur"
    quantity_numeric NUMERIC(12, 3) NOT NULL DEFAULT 1,
    unit_price_cents BIGINT NOT NULL DEFAULT 0,
    tax_rate INTEGER NOT NULL DEFAULT 2100,         -- Basis points: 2100 = 21%, 900 = 9%, 0 = 0%

    is_optional BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Quote counter per organization for quote number generation
CREATE TABLE RAC_quote_counters (
    organization_id UUID PRIMARY KEY REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    last_number INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_quotes_lead ON RAC_quotes(lead_id);
CREATE INDEX idx_quotes_org ON RAC_quotes(organization_id);
CREATE INDEX idx_quote_items_quote ON RAC_quote_items(quote_id);
CREATE UNIQUE INDEX idx_quotes_number_org ON RAC_quotes(organization_id, quote_number);
