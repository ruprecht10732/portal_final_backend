package repository

import (
	"context"
	"fmt"
	"strings"

	isdedb "portal_final_backend/internal/isde/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo implements the ISDE repository using sqlc-generated queries.
type Repo struct {
	pool    *pgxpool.Pool
	queries *isdedb.Queries
}

type measureConfigRow struct {
	measureID            string
	displayName          string
	category             string
	qualifyingGroup      string
	minM2                pgtype.Numeric
	performanceRule      string
	performanceThreshold pgtype.Numeric
	rateMode             string
	requiresPrimaryGlass bool
	legacyMaxFrameUValue pgtype.Numeric
	baseRateCentsPerM2   int64
	upgradedRate         pgtype.Numeric
	maxM2                pgtype.Numeric
	mkiBonusCentsPerM2   int64
}

// New creates a new ISDE repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool, queries: isdedb.New(pool)}
}

// Compile-time check.
var _ Repository = (*Repo)(nil)

func (r *Repo) ListMeasureRulesByIDs(ctx context.Context, measureIDs []string) ([]MeasureRule, error) {
	if r.queries == nil {
		return nil, nil
	}
	if len(measureIDs) == 0 {
		return nil, nil
	}

	rows, err := r.queries.ListMeasureRulesByIDs(ctx, measureIDs)
	if err != nil {
		return nil, fmt.Errorf("list measure rules by ids: %w", err)
	}

	result := make([]MeasureRule, 0, len(rows))
	for _, row := range rows {
		minM2, err := numericFloat64(row.MinM2)
		if err != nil {
			return nil, fmt.Errorf("parse measure rule min_m2 for %s: %w", row.MeasureID, err)
		}
		threshold, err := optionalNumericFloat64(row.PerformanceThreshold)
		if err != nil {
			return nil, fmt.Errorf("parse performance threshold for %s: %w", row.MeasureID, err)
		}
		result = append(result, MeasureRule{
			MeasureID:            strings.TrimSpace(row.MeasureID),
			DisplayName:          strings.TrimSpace(row.DisplayName),
			Category:             strings.TrimSpace(row.Category),
			MinM2:                minM2,
			PerformanceRule:      strings.TrimSpace(row.PerformanceRule),
			PerformanceThreshold: threshold,
			BaseRateCentsPerM2:   row.BaseRateCentsPerM2,
			DoubleRateCentsPerM2: row.DoubleRateCentsPerM2,
			MKIBonusCentsPerM2:   row.MkiBonusCentsPerM2,
		})
	}
	return result, nil
}

func (r *Repo) ListMeasureConfigsByIDsAndYear(ctx context.Context, measureIDs []string, executionYear int) ([]MeasureConfig, error) {
	if r.pool == nil || len(measureIDs) == 0 {
		return nil, nil
	}

	const query = `
SELECT
  d.measure_id,
  d.display_name,
  d.category,
  d.qualifying_group,
  d.min_m2,
  d.performance_rule,
  d.performance_threshold,
  d.rate_mode,
  d.requires_primary_glass,
  d.legacy_max_frame_u_value,
  y.base_rate_cents_per_m2,
  y.upgraded_rate_cents_per_m2,
  y.max_m2,
  y.mki_bonus_cents_per_m2
FROM RAC_isde_measure_definitions d
JOIN RAC_isde_measure_year_rules y
  ON y.measure_id = d.measure_id
 AND y.execution_year = $1
WHERE d.is_active = TRUE
  AND d.measure_id = ANY($2::text[])
ORDER BY d.measure_id`

	rows, err := r.pool.Query(ctx, query, executionYear, measureIDs)
	if err != nil {
		return nil, fmt.Errorf("list measure configs by ids and year: %w", err)
	}
	defer rows.Close()

	result := make([]MeasureConfig, 0, len(measureIDs))
	for rows.Next() {
		row, err := scanMeasureConfigRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan measure config row: %w", err)
		}

		config, err := parseMeasureConfigRow(row)
		if err != nil {
			return nil, err
		}

		result = append(result, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate measure config rows: %w", err)
	}
	return result, nil
}

func (r *Repo) ListInstallationMeldcodesByCodes(ctx context.Context, meldcodes []string) ([]InstallationMeldcode, error) {
	if r.queries == nil {
		return nil, nil
	}
	if len(meldcodes) == 0 {
		return nil, nil
	}

	rows, err := r.queries.ListInstallationMeldcodesByCodes(ctx, meldcodes)
	if err != nil {
		return nil, fmt.Errorf("list installation meldcodes by codes: %w", err)
	}

	result := make([]InstallationMeldcode, 0, len(rows))
	for _, row := range rows {
		result = append(result, InstallationMeldcode{
			Meldcode:           strings.TrimSpace(row.Meldcode),
			Category:           strings.TrimSpace(row.Category),
			Brand:              optionalText(row.Brand),
			ProductName:        optionalText(row.ProductName),
			SubsidyAmountCents: row.SubsidyAmountCents,
		})
	}
	return result, nil
}

func (r *Repo) GetProgramYearRule(ctx context.Context, executionYear int) (ProgramYearRule, error) {
	if r.pool == nil {
		return ProgramYearRule{}, nil
	}

	const query = `
SELECT
  execution_year,
  ventilation_amount_cents,
  warmtenet_amount_cents,
  electric_cooking_amount_cents,
  air_water_start_amount_cents,
  air_water_amount_per_kw_cents,
  air_water_aplusplusplus_bonus_cents,
  air_water_kw_offset
FROM RAC_isde_program_year_rules
WHERE execution_year = $1`

	var rule ProgramYearRule
	var kwOffset pgtype.Numeric
	row := r.pool.QueryRow(ctx, query, executionYear)
	if err := row.Scan(
		&rule.ExecutionYear,
		&rule.VentilationAmountCents,
		&rule.WarmtenetAmountCents,
		&rule.ElectricCookingAmountCents,
		&rule.AirWaterStartAmountCents,
		&rule.AirWaterAmountPerKWCents,
		&rule.AirWaterAPlusPlusPlusBonusCents,
		&kwOffset,
	); err != nil {
		return ProgramYearRule{}, fmt.Errorf("get program year rule: %w", err)
	}
	parsedOffset, err := numericFloat64(kwOffset)
	if err != nil {
		return ProgramYearRule{}, fmt.Errorf("parse air water kw offset: %w", err)
	}
	rule.AirWaterKWOffset = parsedOffset
	return rule, nil
}

func scanMeasureConfigRow(rows pgx.Rows) (measureConfigRow, error) {
	var row measureConfigRow
	err := rows.Scan(
		&row.measureID,
		&row.displayName,
		&row.category,
		&row.qualifyingGroup,
		&row.minM2,
		&row.performanceRule,
		&row.performanceThreshold,
		&row.rateMode,
		&row.requiresPrimaryGlass,
		&row.legacyMaxFrameUValue,
		&row.baseRateCentsPerM2,
		&row.upgradedRate,
		&row.maxM2,
		&row.mkiBonusCentsPerM2,
	)
	return row, err
}

func parseMeasureConfigRow(row measureConfigRow) (MeasureConfig, error) {
	parsedMinM2, err := numericFloat64(row.minM2)
	if err != nil {
		return MeasureConfig{}, fmt.Errorf("parse min m2 for %s: %w", row.measureID, err)
	}
	parsedMaxM2, err := numericFloat64(row.maxM2)
	if err != nil {
		return MeasureConfig{}, fmt.Errorf("parse max m2 for %s: %w", row.measureID, err)
	}
	parsedThreshold, err := optionalNumericFloat64(row.performanceThreshold)
	if err != nil {
		return MeasureConfig{}, fmt.Errorf("parse threshold for %s: %w", row.measureID, err)
	}
	parsedLegacyMaxFrameUValue, err := optionalNumericFloat64(row.legacyMaxFrameUValue)
	if err != nil {
		return MeasureConfig{}, fmt.Errorf("parse legacy max frame U value for %s: %w", row.measureID, err)
	}
	parsedUpgradedRate, err := optionalNumericInt64(row.upgradedRate)
	if err != nil {
		return MeasureConfig{}, fmt.Errorf("parse upgraded rate for %s: %w", row.measureID, err)
	}

	return MeasureConfig{
		MeasureID:              strings.TrimSpace(row.measureID),
		DisplayName:            strings.TrimSpace(row.displayName),
		Category:               strings.TrimSpace(row.category),
		QualifyingGroup:        strings.TrimSpace(row.qualifyingGroup),
		MinM2:                  parsedMinM2,
		PerformanceRule:        strings.TrimSpace(row.performanceRule),
		PerformanceThreshold:   parsedThreshold,
		RateMode:               strings.TrimSpace(row.rateMode),
		BaseRateCentsPerM2:     row.baseRateCentsPerM2,
		UpgradedRateCentsPerM2: parsedUpgradedRate,
		MaxM2:                  parsedMaxM2,
		MKIBonusCentsPerM2:     row.mkiBonusCentsPerM2,
		RequiresPrimaryGlass:   row.requiresPrimaryGlass,
		LegacyMaxFrameUValue:   parsedLegacyMaxFrameUValue,
	}, nil
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := strings.TrimSpace(value.String)
	if text == "" {
		return nil
	}
	return &text
}

func numericFloat64(value pgtype.Numeric) (float64, error) {
	if !value.Valid {
		return 0, nil
	}
	floatValue, err := value.Float64Value()
	if err != nil {
		return 0, err
	}
	if !floatValue.Valid {
		return 0, nil
	}
	return floatValue.Float64, nil
}

func optionalNumericFloat64(value pgtype.Numeric) (*float64, error) {
	if !value.Valid {
		return nil, nil
	}
	floatValue, err := value.Float64Value()
	if err != nil {
		return nil, err
	}
	if !floatValue.Valid {
		return nil, nil
	}
	result := floatValue.Float64
	return &result, nil
}

func optionalNumericInt64(value pgtype.Numeric) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	intValue, err := value.Int64Value()
	if err != nil {
		return nil, err
	}
	if !intValue.Valid {
		return nil, nil
	}
	result := intValue.Int64
	return &result, nil
}
