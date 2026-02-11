-- +goose Up
-- 026_catalog_core.sql
-- Catalog core tables: VAT rates, products, and product-material links.

CREATE TABLE IF NOT EXISTS RAC_catalog_vat_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    rate_bps INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_vat_rates_org_name
    ON RAC_catalog_vat_rates(organization_id, name);

CREATE TABLE IF NOT EXISTS RAC_catalog_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vat_rate_id UUID NOT NULL REFERENCES RAC_catalog_vat_rates(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    reference TEXT NOT NULL,
    description TEXT,
    price_cents BIGINT NOT NULL DEFAULT 0,
    type TEXT NOT NULL,
    period_count INTEGER,
    period_unit TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_catalog_products_org_id
    ON RAC_catalog_products(organization_id);

CREATE INDEX IF NOT EXISTS idx_catalog_products_vat_rate_id
    ON RAC_catalog_products(vat_rate_id);

CREATE INDEX IF NOT EXISTS idx_catalog_products_type
    ON RAC_catalog_products(type);

CREATE TABLE IF NOT EXISTS RAC_catalog_product_materials (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES RAC_catalog_products(id) ON DELETE CASCADE,
    material_id UUID NOT NULL REFERENCES RAC_catalog_products(id) ON DELETE CASCADE,
    PRIMARY KEY (organization_id, product_id, material_id)
);

CREATE INDEX IF NOT EXISTS idx_catalog_product_materials_product
    ON RAC_catalog_product_materials(product_id);

CREATE INDEX IF NOT EXISTS idx_catalog_product_materials_material
    ON RAC_catalog_product_materials(material_id);
