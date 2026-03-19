-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_isde_measure_rules (
  measure_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('insulation', 'glass')),
  min_m2 NUMERIC(10,2) NOT NULL CHECK (min_m2 >= 0),
  performance_rule TEXT NOT NULL CHECK (performance_rule IN ('none', 'rd_min', 'u_max')),
  performance_threshold NUMERIC(10,3),
  base_rate_cents_per_m2 BIGINT NOT NULL CHECK (base_rate_cents_per_m2 >= 0),
  double_rate_cents_per_m2 BIGINT NOT NULL CHECK (double_rate_cents_per_m2 >= 0),
  mki_bonus_cents_per_m2 BIGINT NOT NULL DEFAULT 0 CHECK (mki_bonus_cents_per_m2 >= 0),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT isde_measure_threshold_consistency CHECK (
    (performance_rule = 'none' AND performance_threshold IS NULL)
    OR
    (performance_rule IN ('rd_min', 'u_max') AND performance_threshold IS NOT NULL)
  )
);

CREATE TABLE IF NOT EXISTS RAC_isde_installation_meldcodes (
  meldcode TEXT PRIMARY KEY,
  category TEXT NOT NULL CHECK (category IN ('heat_pump', 'solar_boiler')),
  brand TEXT,
  product_name TEXT,
  subsidy_amount_cents BIGINT NOT NULL CHECK (subsidy_amount_cents >= 0),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_isde_measure_rules_category_active
  ON RAC_isde_measure_rules (category, is_active);

CREATE INDEX IF NOT EXISTS idx_isde_installation_meldcodes_category_active
  ON RAC_isde_installation_meldcodes (category, is_active);

INSERT INTO RAC_isde_measure_rules (
  measure_id,
  display_name,
  category,
  min_m2,
  performance_rule,
  performance_threshold,
  base_rate_cents_per_m2,
  double_rate_cents_per_m2,
  mki_bonus_cents_per_m2,
  is_active
) VALUES
  ('roof', 'Dakisolatie', 'insulation', 20.0, 'rd_min', 3.5, 1500, 3000, 500, TRUE),
  ('attic', 'Zolder-/vlieringvloerisolatie', 'insulation', 20.0, 'rd_min', 3.5, 400, 800, 150, TRUE),
  ('facade', 'Gevelisolatie', 'insulation', 10.0, 'rd_min', 3.5, 1900, 3800, 600, TRUE),
  ('cavity_wall', 'Spouwmuurisolatie', 'insulation', 10.0, 'rd_min', 1.1, 400, 800, 150, TRUE),
  ('floor', 'Vloerisolatie', 'insulation', 20.0, 'rd_min', 3.5, 550, 1100, 200, TRUE),
  ('crawl_space', 'Bodemisolatie', 'insulation', 20.0, 'rd_min', 3.5, 300, 600, 100, TRUE),
  ('hr_plus_plus', 'HR++ glas', 'glass', 8.0, 'u_max', 1.2, 2300, 4600, 0, TRUE),
  ('triple_glass', 'Triple glas', 'glass', 8.0, 'u_max', 0.7, 6550, 13100, 0, TRUE)
ON CONFLICT (measure_id) DO UPDATE
SET
  display_name = EXCLUDED.display_name,
  category = EXCLUDED.category,
  min_m2 = EXCLUDED.min_m2,
  performance_rule = EXCLUDED.performance_rule,
  performance_threshold = EXCLUDED.performance_threshold,
  base_rate_cents_per_m2 = EXCLUDED.base_rate_cents_per_m2,
  double_rate_cents_per_m2 = EXCLUDED.double_rate_cents_per_m2,
  mki_bonus_cents_per_m2 = EXCLUDED.mki_bonus_cents_per_m2,
  is_active = EXCLUDED.is_active,
  updated_at = now();

INSERT INTO RAC_isde_installation_meldcodes (
  meldcode,
  category,
  brand,
  product_name,
  subsidy_amount_cents,
  is_active
) VALUES
  ('KA00001', 'heat_pump', 'BrandX', 'Water-water 71kW A++', 1290000, TRUE),
  ('ZB00001', 'solar_boiler', 'BrandY', 'Zonneboiler <= 5m2', 175000, TRUE),
  ('ZB00002', 'solar_boiler', 'BrandZ', 'Zonneboiler > 5m2', 205000, TRUE)
ON CONFLICT (meldcode) DO UPDATE
SET
  category = EXCLUDED.category,
  brand = EXCLUDED.brand,
  product_name = EXCLUDED.product_name,
  subsidy_amount_cents = EXCLUDED.subsidy_amount_cents,
  is_active = EXCLUDED.is_active,
  updated_at = now();

-- +goose Down
DROP TABLE IF EXISTS RAC_isde_installation_meldcodes;
DROP TABLE IF EXISTS RAC_isde_measure_rules;
