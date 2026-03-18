-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_partner_offer_terms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_by UUID REFERENCES RAC_users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_partner_offer_terms_org_version
    ON RAC_partner_offer_terms (organization_id, version);

CREATE UNIQUE INDEX IF NOT EXISTS idx_partner_offer_terms_active_per_org
    ON RAC_partner_offer_terms (organization_id)
    WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_partner_offer_terms_org_created_at
    ON RAC_partner_offer_terms (organization_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_partner_offer_terms_org_created_at;
DROP INDEX IF EXISTS idx_partner_offer_terms_active_per_org;
DROP INDEX IF EXISTS idx_partner_offer_terms_org_version;
DROP TABLE IF EXISTS RAC_partner_offer_terms;