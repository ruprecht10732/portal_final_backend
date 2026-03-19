-- ISDE Domain SQL Queries

-- name: ListMeasureRulesByIDs :many
SELECT
  measure_id,
  display_name,
  category,
  min_m2,
  performance_rule,
  performance_threshold,
  base_rate_cents_per_m2,
  double_rate_cents_per_m2,
  mki_bonus_cents_per_m2
FROM RAC_isde_measure_rules
WHERE is_active = TRUE
  AND measure_id = ANY(sqlc.arg(measure_ids)::text[]);

-- name: ListInstallationMeldcodesByCodes :many
SELECT
  meldcode,
  category,
  brand,
  product_name,
  subsidy_amount_cents
FROM RAC_isde_installation_meldcodes
WHERE is_active = TRUE
  AND meldcode = ANY(sqlc.arg(meldcodes)::text[]);
