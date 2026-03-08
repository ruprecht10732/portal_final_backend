-- +goose Up
CREATE TABLE RAC_quote_pricing_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    service_type VARCHAR(100),
    postcode_raw TEXT,
    postcode_prefix_zip4 VARCHAR(4),
    source_type TEXT NOT NULL,
    quote_revision INTEGER NOT NULL,
    pricing_mode TEXT NOT NULL,
    discount_type TEXT NOT NULL,
    discount_value BIGINT NOT NULL DEFAULT 0,
    material_subtotal_cents BIGINT,
    labor_subtotal_low_cents BIGINT,
    labor_subtotal_high_cents BIGINT,
    extra_costs_cents BIGINT,
    subtotal_cents BIGINT NOT NULL,
    discount_amount_cents BIGINT NOT NULL,
    tax_total_cents BIGINT NOT NULL,
    total_cents BIGINT NOT NULL,
    item_count INTEGER NOT NULL,
    catalog_item_count INTEGER NOT NULL DEFAULT 0,
    ad_hoc_item_count INTEGER NOT NULL DEFAULT 0,
    structured_items JSONB NOT NULL DEFAULT '[]'::jsonb,
    notes TEXT,
    price_range_text TEXT,
    scope_text TEXT,
    estimator_run_id TEXT,
    model_name TEXT,
    created_by_actor TEXT NOT NULL,
    created_by_user_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (quote_id, quote_revision)
);

CREATE INDEX idx_quote_pricing_snapshots_quote_revision
    ON RAC_quote_pricing_snapshots (quote_id, quote_revision DESC);

CREATE INDEX idx_quote_pricing_snapshots_org_service_zip_created
    ON RAC_quote_pricing_snapshots (organization_id, service_type, postcode_prefix_zip4, created_at DESC);

CREATE TABLE RAC_quote_pricing_outcomes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    snapshot_id UUID REFERENCES RAC_quote_pricing_snapshots(id) ON DELETE SET NULL,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    outcome_type TEXT NOT NULL,
    rejection_reason TEXT,
    accepted_total_cents BIGINT,
    final_total_cents BIGINT,
    outcome_at TIMESTAMPTZ NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quote_pricing_outcomes_quote_created
    ON RAC_quote_pricing_outcomes (quote_id, created_at DESC);

CREATE INDEX idx_quote_pricing_outcomes_org_outcome_created
    ON RAC_quote_pricing_outcomes (organization_id, outcome_type, created_at DESC);

CREATE INDEX idx_quote_pricing_outcomes_snapshot_id
    ON RAC_quote_pricing_outcomes (snapshot_id)
    WHERE snapshot_id IS NOT NULL;

CREATE TABLE RAC_quote_pricing_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES RAC_quotes(id) ON DELETE CASCADE,
    snapshot_id UUID REFERENCES RAC_quote_pricing_snapshots(id) ON DELETE SET NULL,
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    quote_item_id UUID,
    field_name TEXT NOT NULL,
    ai_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    human_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    delta_cents BIGINT,
    delta_percentage DOUBLE PRECISION,
    reason TEXT,
    ai_finding_code TEXT,
    created_by_user_id UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quote_pricing_corrections_quote_created
    ON RAC_quote_pricing_corrections (quote_id, created_at DESC);

CREATE INDEX idx_quote_pricing_corrections_org_field_created
    ON RAC_quote_pricing_corrections (organization_id, field_name, created_at DESC);

CREATE INDEX idx_quote_pricing_corrections_snapshot_id
    ON RAC_quote_pricing_corrections (snapshot_id)
    WHERE snapshot_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS RAC_quote_pricing_corrections;
DROP TABLE IF EXISTS RAC_quote_pricing_outcomes;
DROP TABLE IF EXISTS RAC_quote_pricing_snapshots;