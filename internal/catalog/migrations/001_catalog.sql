-- Migration: Create catalog tables for products and VAT rates

CREATE TABLE IF NOT EXISTS catalog_vat_rates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  rate_bps INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_vat_rates_org_name
  ON catalog_vat_rates(organization_id, name);

CREATE INDEX IF NOT EXISTS idx_catalog_vat_rates_org
  ON catalog_vat_rates(organization_id);

CREATE TABLE IF NOT EXISTS catalog_products (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id),
  vat_rate_id UUID NOT NULL REFERENCES catalog_vat_rates(id),
  title TEXT NOT NULL,
  reference TEXT NOT NULL,
  description TEXT,
  price_cents INTEGER NOT NULL,
  type TEXT NOT NULL,
  period_count INTEGER,
  period_unit TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT catalog_products_type_check
    CHECK (type IN ('digital_service', 'service', 'product', 'material')),
  CONSTRAINT catalog_products_period_check
    CHECK (
      (period_count IS NULL AND period_unit IS NULL)
      OR
      (period_count IS NOT NULL AND period_count > 0 AND period_unit IN ('day', 'week', 'month', 'quarter', 'year'))
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_org_reference
  ON catalog_products(organization_id, reference);

CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_id_org
  ON catalog_products(id, organization_id);

CREATE INDEX IF NOT EXISTS idx_catalog_products_org
  ON catalog_products(organization_id);

CREATE INDEX IF NOT EXISTS idx_catalog_products_org_type
  ON catalog_products(organization_id, type);

CREATE INDEX IF NOT EXISTS idx_catalog_products_org_vat
  ON catalog_products(organization_id, vat_rate_id);

CREATE TABLE IF NOT EXISTS catalog_product_materials (
  organization_id UUID NOT NULL REFERENCES organizations(id),
  product_id UUID NOT NULL,
  material_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, product_id, material_id),
  CONSTRAINT catalog_product_materials_product_fk
    FOREIGN KEY (product_id, organization_id)
    REFERENCES catalog_products(id, organization_id)
    ON DELETE CASCADE,
  CONSTRAINT catalog_product_materials_material_fk
    FOREIGN KEY (material_id, organization_id)
    REFERENCES catalog_products(id, organization_id)
    ON DELETE CASCADE,
  CONSTRAINT catalog_product_materials_no_self
    CHECK (product_id <> material_id)
);

CREATE INDEX IF NOT EXISTS idx_catalog_product_materials_product
  ON catalog_product_materials(organization_id, product_id);

CREATE INDEX IF NOT EXISTS idx_catalog_product_materials_material
  ON catalog_product_materials(organization_id, material_id);
