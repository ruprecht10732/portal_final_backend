package repository

import "context"

// MeasureRule represents one active ISDE rule row for insulation or glass.
type MeasureRule struct {
	MeasureID            string
	DisplayName          string
	Category             string
	MinM2                float64
	PerformanceRule      string
	PerformanceThreshold *float64
	BaseRateCentsPerM2   int64
	DoubleRateCentsPerM2 int64
	MKIBonusCentsPerM2   int64
}

// MeasureConfig represents the normalized year-specific config used by the ISDE engine.
type MeasureConfig struct {
	MeasureID               string
	DisplayName             string
	Category                string
	QualifyingGroup         string
	MinM2                   float64
	PerformanceRule         string
	PerformanceThreshold    *float64
	RateMode                string
	BaseRateCentsPerM2      int64
	UpgradedRateCentsPerM2  *int64
	MaxM2                   float64
	MKIBonusCentsPerM2      int64
	RequiresPrimaryGlass    bool
	LegacyMaxFrameUValue    *float64
}

// ProgramYearRule holds year-specific flat amounts and air-water heat pump parameters.
type ProgramYearRule struct {
	ExecutionYear                     int
	VentilationAmountCents           int64
	WarmtenetAmountCents             int64
	ElectricCookingAmountCents       int64
	AirWaterStartAmountCents         int64
	AirWaterAmountPerKWCents         int64
	AirWaterAPlusPlusPlusBonusCents  int64
	AirWaterKWOffset                 float64
}

// InstallationMeldcode represents one active installation subsidy lookup row.
type InstallationMeldcode struct {
	Meldcode           string
	Category           string
	Brand              *string
	ProductName        *string
	SubsidyAmountCents int64
}

// Repository provides read access to ISDE rule tables.
type Repository interface {
	ListMeasureRulesByIDs(ctx context.Context, measureIDs []string) ([]MeasureRule, error)
	ListMeasureConfigsByIDsAndYear(ctx context.Context, measureIDs []string, executionYear int) ([]MeasureConfig, error)
	ListInstallationMeldcodesByCodes(ctx context.Context, meldcodes []string) ([]InstallationMeldcode, error)
	GetProgramYearRule(ctx context.Context, executionYear int) (ProgramYearRule, error)
}
