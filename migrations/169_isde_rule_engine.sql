-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_isde_measure_definitions (
  measure_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('insulation', 'glass')),
  qualifying_group TEXT NOT NULL,
  min_m2 NUMERIC(10,2) NOT NULL CHECK (min_m2 >= 0),
  performance_rule TEXT NOT NULL CHECK (performance_rule IN ('none', 'rd_min', 'u_max')),
  performance_threshold NUMERIC(10,3),
  rate_mode TEXT NOT NULL CHECK (rate_mode IN ('standard', 'upgraded_frame')),
  requires_primary_glass BOOLEAN NOT NULL DEFAULT FALSE,
  legacy_max_frame_u_value NUMERIC(10,3),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT isde_measure_definitions_threshold_consistency CHECK (
    (performance_rule = 'none' AND performance_threshold IS NULL)
    OR
    (performance_rule IN ('rd_min', 'u_max') AND performance_threshold IS NOT NULL)
  )
);

CREATE TABLE IF NOT EXISTS RAC_isde_measure_year_rules (
  measure_id TEXT NOT NULL REFERENCES RAC_isde_measure_definitions(measure_id) ON DELETE CASCADE,
  execution_year INTEGER NOT NULL CHECK (execution_year >= 2024),
  base_rate_cents_per_m2 BIGINT NOT NULL CHECK (base_rate_cents_per_m2 >= 0),
  upgraded_rate_cents_per_m2 BIGINT,
  max_m2 NUMERIC(10,2) NOT NULL CHECK (max_m2 >= 0),
  mki_bonus_cents_per_m2 BIGINT NOT NULL DEFAULT 0 CHECK (mki_bonus_cents_per_m2 >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (measure_id, execution_year),
  CONSTRAINT isde_measure_year_rules_upgraded_rate_consistency CHECK (
    upgraded_rate_cents_per_m2 IS NULL OR upgraded_rate_cents_per_m2 >= 0
  )
);

CREATE TABLE IF NOT EXISTS RAC_isde_program_year_rules (
  execution_year INTEGER PRIMARY KEY CHECK (execution_year >= 2024),
  ventilation_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (ventilation_amount_cents >= 0),
  warmtenet_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (warmtenet_amount_cents >= 0),
  electric_cooking_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (electric_cooking_amount_cents >= 0),
  air_water_start_amount_cents BIGINT NOT NULL CHECK (air_water_start_amount_cents >= 0),
  air_water_amount_per_kw_cents BIGINT NOT NULL CHECK (air_water_amount_per_kw_cents >= 0),
  air_water_aplusplusplus_bonus_cents BIGINT NOT NULL CHECK (air_water_aplusplusplus_bonus_cents >= 0),
  air_water_kw_offset NUMERIC(10,2) NOT NULL CHECK (air_water_kw_offset >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_isde_measure_definitions_active
  ON RAC_isde_measure_definitions (is_active, category);

INSERT INTO RAC_isde_measure_definitions (
  measure_id,
  display_name,
  category,
  qualifying_group,
  min_m2,
  performance_rule,
  performance_threshold,
  rate_mode,
  requires_primary_glass,
  legacy_max_frame_u_value,
  is_active
) VALUES
  ('roof', 'Dakisolatie', 'insulation', 'insulation_roof_attic', 20.0, 'rd_min', 3.5, 'standard', FALSE, NULL, TRUE),
  ('attic', 'Zolder-/vlieringvloerisolatie', 'insulation', 'insulation_roof_attic', 20.0, 'rd_min', 3.5, 'standard', FALSE, NULL, TRUE),
  ('facade', 'Gevelisolatie', 'insulation', 'insulation_facade', 10.0, 'rd_min', 3.5, 'standard', FALSE, NULL, TRUE),
  ('cavity_wall', 'Spouwmuurisolatie', 'insulation', 'insulation_cavity_wall', 10.0, 'rd_min', 1.1, 'standard', FALSE, NULL, TRUE),
  ('floor', 'Vloerisolatie', 'insulation', 'insulation_floor_crawl_space', 20.0, 'rd_min', 3.5, 'standard', FALSE, NULL, TRUE),
  ('crawl_space', 'Bodemisolatie', 'insulation', 'insulation_floor_crawl_space', 20.0, 'rd_min', 3.5, 'standard', FALSE, NULL, TRUE),
  ('hr_plus_plus', 'HR++ glas', 'glass', 'glass', 0, 'u_max', 1.2, 'standard', FALSE, NULL, TRUE),
  ('triple_glass', 'Triple glas', 'glass', 'glass', 0, 'u_max', 0.7, 'upgraded_frame', FALSE, 1.5, TRUE),
  ('vacuum_glass', 'Vacuumglas', 'glass', 'glass', 0, 'u_max', 0.7, 'upgraded_frame', FALSE, 1.5, TRUE),
  ('glass_panel_low', 'Isolerend paneel', 'glass', 'glass', 0, 'u_max', 1.2, 'standard', TRUE, NULL, TRUE),
  ('glass_panel_high', 'Isolerend paneel hoogwaardig', 'glass', 'glass', 0, 'u_max', 0.7, 'standard', TRUE, NULL, TRUE),
  ('insulated_door_low', 'Isolerende deur', 'glass', 'glass', 0, 'u_max', 1.5, 'standard', TRUE, NULL, TRUE),
  ('insulated_door_high', 'Isolerende deur hoogwaardig', 'glass', 'glass', 0, 'u_max', 1.0, 'upgraded_frame', TRUE, 1.5, TRUE)
ON CONFLICT (measure_id) DO UPDATE
SET
  display_name = EXCLUDED.display_name,
  category = EXCLUDED.category,
  qualifying_group = EXCLUDED.qualifying_group,
  min_m2 = EXCLUDED.min_m2,
  performance_rule = EXCLUDED.performance_rule,
  performance_threshold = EXCLUDED.performance_threshold,
  rate_mode = EXCLUDED.rate_mode,
  requires_primary_glass = EXCLUDED.requires_primary_glass,
  legacy_max_frame_u_value = EXCLUDED.legacy_max_frame_u_value,
  is_active = EXCLUDED.is_active,
  updated_at = now();

INSERT INTO RAC_isde_measure_year_rules (
  measure_id,
  execution_year,
  base_rate_cents_per_m2,
  upgraded_rate_cents_per_m2,
  max_m2,
  mki_bonus_cents_per_m2
) VALUES
  ('roof', 2024, 1500, NULL, 200.0, 500),
  ('roof', 2025, 1625, NULL, 200.0, 500),
  ('roof', 2026, 1625, NULL, 200.0, 500),
  ('attic', 2024, 400, NULL, 130.0, 150),
  ('attic', 2025, 400, NULL, 200.0, 150),
  ('attic', 2026, 400, NULL, 200.0, 150),
  ('facade', 2024, 1900, NULL, 170.0, 600),
  ('facade', 2025, 2025, NULL, 170.0, 600),
  ('facade', 2026, 2025, NULL, 170.0, 600),
  ('cavity_wall', 2024, 400, NULL, 170.0, 150),
  ('cavity_wall', 2025, 525, NULL, 170.0, 150),
  ('cavity_wall', 2026, 525, NULL, 170.0, 150),
  ('floor', 2024, 550, NULL, 130.0, 200),
  ('floor', 2025, 550, NULL, 130.0, 200),
  ('floor', 2026, 550, NULL, 130.0, 200),
  ('crawl_space', 2024, 300, NULL, 130.0, 100),
  ('crawl_space', 2025, 300, NULL, 130.0, 100),
  ('crawl_space', 2026, 300, NULL, 130.0, 100),
  ('hr_plus_plus', 2024, 2300, NULL, 45.0, 0),
  ('hr_plus_plus', 2025, 2500, NULL, 45.0, 0),
  ('hr_plus_plus', 2026, 2500, NULL, 45.0, 0),
  ('triple_glass', 2024, 2300, 6550, 45.0, 0),
  ('triple_glass', 2025, 2500, 11100, 45.0, 0),
  ('triple_glass', 2026, 2500, 11100, 45.0, 0),
  ('vacuum_glass', 2024, 2300, 6550, 45.0, 0),
  ('vacuum_glass', 2025, 2500, 11100, 45.0, 0),
  ('vacuum_glass', 2026, 2500, 11100, 45.0, 0),
  ('glass_panel_low', 2024, 1000, NULL, 45.0, 0),
  ('glass_panel_low', 2025, 1000, NULL, 45.0, 0),
  ('glass_panel_low', 2026, 1000, NULL, 45.0, 0),
  ('glass_panel_high', 2024, 4500, NULL, 45.0, 0),
  ('glass_panel_high', 2025, 4500, NULL, 45.0, 0),
  ('glass_panel_high', 2026, 4500, NULL, 45.0, 0),
  ('insulated_door_low', 2024, 2300, NULL, 45.0, 0),
  ('insulated_door_low', 2025, 2500, NULL, 45.0, 0),
  ('insulated_door_low', 2026, 2500, NULL, 45.0, 0),
  ('insulated_door_high', 2024, 2300, 6550, 45.0, 0),
  ('insulated_door_high', 2025, 2500, 11100, 45.0, 0),
  ('insulated_door_high', 2026, 2500, 11100, 45.0, 0)
ON CONFLICT (measure_id, execution_year) DO UPDATE
SET
  base_rate_cents_per_m2 = EXCLUDED.base_rate_cents_per_m2,
  upgraded_rate_cents_per_m2 = EXCLUDED.upgraded_rate_cents_per_m2,
  max_m2 = EXCLUDED.max_m2,
  mki_bonus_cents_per_m2 = EXCLUDED.mki_bonus_cents_per_m2,
  updated_at = now();

INSERT INTO RAC_isde_program_year_rules (
  execution_year,
  ventilation_amount_cents,
  warmtenet_amount_cents,
  electric_cooking_amount_cents,
  air_water_start_amount_cents,
  air_water_amount_per_kw_cents,
  air_water_aplusplusplus_bonus_cents,
  air_water_kw_offset
) VALUES
  (2024, 0, 377500, 40000, 210000, 15000, 22500, 1.0),
  (2025, 0, 377500, 40000, 125000, 22500, 20000, 1.0),
  (2026, 40000, 377500, 40000, 102500, 22500, 20000, 0.0)
ON CONFLICT (execution_year) DO UPDATE
SET
  ventilation_amount_cents = EXCLUDED.ventilation_amount_cents,
  warmtenet_amount_cents = EXCLUDED.warmtenet_amount_cents,
  electric_cooking_amount_cents = EXCLUDED.electric_cooking_amount_cents,
  air_water_start_amount_cents = EXCLUDED.air_water_start_amount_cents,
  air_water_amount_per_kw_cents = EXCLUDED.air_water_amount_per_kw_cents,
  air_water_aplusplusplus_bonus_cents = EXCLUDED.air_water_aplusplusplus_bonus_cents,
  air_water_kw_offset = EXCLUDED.air_water_kw_offset,
  updated_at = now();

-- +goose Down
DROP TABLE IF EXISTS RAC_isde_program_year_rules;
DROP TABLE IF EXISTS RAC_isde_measure_year_rules;
DROP TABLE IF EXISTS RAC_isde_measure_definitions;