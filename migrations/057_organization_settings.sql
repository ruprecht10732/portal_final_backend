-- +goose Up
-- Organization-level settings for quote defaults (payment terms, validity period).
CREATE TABLE IF NOT EXISTS RAC_organization_settings (
    organization_id UUID PRIMARY KEY REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    quote_payment_days INT NOT NULL DEFAULT 7,
    quote_valid_days   INT NOT NULL DEFAULT 14,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed a row for every existing organization so queries never return empty.
INSERT INTO RAC_organization_settings (organization_id)
SELECT id FROM RAC_organizations
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS RAC_organization_settings;
