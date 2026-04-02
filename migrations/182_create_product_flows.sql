-- +goose Up
-- +goose StatementBegin

CREATE TABLE rac_product_flows (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        REFERENCES rac_organizations(id) ON DELETE CASCADE,
    product_group_id TEXT       NOT NULL,
    version         INTEGER     NOT NULL DEFAULT 1,
    is_active       BOOLEAN     NOT NULL DEFAULT true,
    definition      JSONB       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE rac_product_flows IS 'Server-driven UI flow definitions for product intake wizards';
COMMENT ON COLUMN rac_product_flows.organization_id IS 'NULL = global default, non-NULL = tenant override';
COMMENT ON COLUMN rac_product_flows.definition IS 'Full FlowDefinition JSON: steps, reviewTemplate, payloadSchema';

CREATE INDEX idx_rac_product_flows_org_group
    ON rac_product_flows (organization_id, product_group_id);

-- Only one active global default per product group
CREATE UNIQUE INDEX idx_rac_product_flows_global_active
    ON rac_product_flows (product_group_id)
    WHERE organization_id IS NULL AND is_active = true;

-- Only one active override per org + product group
CREATE UNIQUE INDEX idx_rac_product_flows_org_active
    ON rac_product_flows (organization_id, product_group_id)
    WHERE is_active = true;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rac_product_flows;
-- +goose StatementEnd
