package service

import (
	"context"
	"testing"

	"portal_final_backend/internal/isde/repository"
	"portal_final_backend/internal/isde/transport"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type testRepo struct {
	rules         []repository.MeasureRule
	installations []repository.InstallationMeldcode
}

const errCalculateFmt = "Calculate returned error: %v"

func (r testRepo) ListMeasureConfigsByIDsAndYear(_ context.Context, measureIDs []string, executionYear int) ([]repository.MeasureConfig, error) {
	allowed := make(map[string]struct{}, len(measureIDs))
	for _, id := range measureIDs {
		allowed[id] = struct{}{}
	}
	all := defaultTestMeasureConfigs(executionYear)
	result := make([]repository.MeasureConfig, 0, len(measureIDs))
	for _, config := range all {
		if _, ok := allowed[config.MeasureID]; ok {
			result = append(result, config)
		}
	}
	return result, nil
}

func (r testRepo) GetProgramYearRule(_ context.Context, executionYear int) (repository.ProgramYearRule, error) {
	return defaultTestProgramYearRule(executionYear), nil
}

func (r testRepo) ListMeasureRulesByIDs(_ context.Context, measureIDs []string) ([]repository.MeasureRule, error) {
	allowed := make(map[string]struct{}, len(measureIDs))
	for _, id := range measureIDs {
		allowed[id] = struct{}{}
	}
	result := make([]repository.MeasureRule, 0)
	for _, rule := range r.rules {
		if _, ok := allowed[rule.MeasureID]; ok {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (r testRepo) ListInstallationMeldcodesByCodes(_ context.Context, meldcodes []string) ([]repository.InstallationMeldcode, error) {
	allowed := make(map[string]struct{}, len(meldcodes))
	for _, code := range meldcodes {
		allowed[code] = struct{}{}
	}
	result := make([]repository.InstallationMeldcode, 0)
	for _, installation := range r.installations {
		if _, ok := allowed[installation.Meldcode]; ok {
			result = append(result, installation)
		}
	}
	return result, nil
}

func TestCalculateSingleMeasureUsesBaseRate(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "roof",
			AreaM2:           20,
			PerformanceValue: floatPtr(4.0),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.IsDoubled {
		t.Fatal("expected single measure to not be doubled")
	}
	if resp.TotalAmountCents != 32500 {
		t.Fatalf("expected total 32500 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateTwoMeasuresTriggersDoubledRate(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{
			{MeasureID: "roof", AreaM2: 20, PerformanceValue: floatPtr(4.0)},
			{MeasureID: "floor", AreaM2: 20, PerformanceValue: floatPtr(4.0)},
		},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if !resp.IsDoubled {
		t.Fatal("expected doubled rates for two qualifying measures")
	}
	if resp.TotalAmountCents != 87000 {
		t.Fatalf("expected total 87000 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculatePreviousSubsidyWithin24MonthsTriggersDoubling(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear:                   intPtr(2025),
		PreviousSubsidiesWithin24Months: true,
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "roof",
			AreaM2:           20,
			PerformanceValue: floatPtr(3.8),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if !resp.IsDoubled {
		t.Fatal("expected previous subsidy to count towards doubling")
	}
	if resp.TotalAmountCents != 65000 {
		t.Fatalf("expected total 65000 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateAppliesMKIBonus(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "roof",
			AreaM2:           20,
			PerformanceValue: floatPtr(3.8),
			HasMKIBonus:      true,
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if len(resp.InsulationBreakdown) != 1 {
		t.Fatalf("expected one insulation row, got %d", len(resp.InsulationBreakdown))
	}
	if resp.InsulationBreakdown[0].AmountCents != 42500 {
		t.Fatalf("expected MKI adjusted amount 42500 cents, got %d", resp.InsulationBreakdown[0].AmountCents)
	}
}

func TestCalculateMKIBonusIsNotDoubled(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{
			{MeasureID: "roof", AreaM2: 20, PerformanceValue: floatPtr(3.8), HasMKIBonus: true},
			{MeasureID: "floor", AreaM2: 20, PerformanceValue: floatPtr(3.8)},
		},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if !resp.IsDoubled {
		t.Fatal("expected doubled rates when two categories qualify")
	}
	if resp.TotalAmountCents != 97000 {
		t.Fatalf("expected total 97000 cents with non-doubled MKI bonus, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateRoofAndAtticCountAsSingleCategory(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{
			{MeasureID: "roof", AreaM2: 20, PerformanceValue: floatPtr(3.8)},
			{MeasureID: "attic", AreaM2: 20, PerformanceValue: floatPtr(3.8)},
		},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.IsDoubled {
		t.Fatal("expected roof and attic to count as one category and not trigger doubling")
	}
	if resp.EligibleMeasureCount != 1 {
		t.Fatalf("expected one eligible category, got %d", resp.EligibleMeasureCount)
	}
	if resp.TotalAmountCents != 40500 {
		t.Fatalf("expected total 40500 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateDuplicateInstallationMeldcodeOnlyCountsOnce(t *testing.T) {
	repo := testRepo{installations: []repository.InstallationMeldcode{{
		Meldcode:           "KA00001",
		Category:           "heat_pump",
		SubsidyAmountCents: 1290000,
	}}}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		Installations: []transport.RequestedInstallation{
			{Meldcode: "KA00001"},
			{Meldcode: "KA00001"},
		},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if len(resp.Installations) != 1 {
		t.Fatalf("expected one installation line after dedupe, got %d", len(resp.Installations))
	}
	if resp.TotalAmountCents != 1290000 {
		t.Fatalf("expected total 1290000 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateGlassRuleAndInstallationLookup(t *testing.T) {
	repo := testRepo{
		installations: []repository.InstallationMeldcode{{
			Meldcode:           "KA00001",
			Category:           "heat_pump",
			SubsidyAmountCents: 1290000,
		}},
	}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "hr_plus_plus",
			AreaM2:           10,
			PerformanceValue: floatPtr(1.1),
		}},
		Installations: []transport.RequestedInstallation{{Meldcode: "KA00001"}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if len(resp.GlassBreakdown) != 1 {
		t.Fatalf("expected one glass row, got %d", len(resp.GlassBreakdown))
	}
	if len(resp.Installations) != 1 {
		t.Fatalf("expected one installation row, got %d", len(resp.Installations))
	}
	if resp.TotalAmountCents != 1340000 {
		t.Fatalf("expected total 1340000 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateRejectsMeasureWhenThresholdNotMet(t *testing.T) {
	repo := testRepo{}
	svc := New(repo, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "roof",
			AreaM2:           19.9,
			PerformanceValue: floatPtr(3.6),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 0 {
		t.Fatalf("expected total 0 when min area fails, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateReturnsValidationMessageForGlassThresholdFailure(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2026),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "hr_plus_plus",
			AreaM2:           15,
			PerformanceValue: floatPtr(1.6),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 0 {
		t.Fatalf("expected total 0 for invalid HR++ U-value, got %d", resp.TotalAmountCents)
	}
	if len(resp.ValidationMessages) != 1 {
		t.Fatalf("expected one validation message, got %d", len(resp.ValidationMessages))
	}
	if resp.ValidationMessages[0] != "HR++ glas vereist een waarde van 1.2 of lager." {
		t.Fatalf("unexpected validation message: %s", resp.ValidationMessages[0])
	}
}

func TestCalculateGlassPanelsRequirePrimaryGlass(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "glass_panel_low",
			AreaM2:           8,
			PerformanceValue: floatPtr(1.1),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 0 {
		t.Fatalf("expected panel without primary glass to be rejected, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateTripleGlassFallsBackWithoutFrameReplacement(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2025),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "triple_glass",
			AreaM2:           10,
			PerformanceValue: floatPtr(0.6),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 25000 {
		t.Fatalf("expected triple glass without frame replacement to use HR++ rate, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateVentilationDoesNotTriggerDoubling(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2026),
		Measures: []transport.RequestedMeasure{{
			MeasureID:        "roof",
			AreaM2:           20,
			PerformanceValue: floatPtr(3.8),
		}},
		Installations: []transport.RequestedInstallation{{Kind: "ventilation"}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.IsDoubled {
		t.Fatal("expected ventilation not to trigger doubling")
	}
	if resp.TotalAmountCents != 72500 {
		t.Fatalf("expected roof plus ventilation total 72500 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateWarmtenetBlocksElectricCooking(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2026),
		Installations: []transport.RequestedInstallation{
			{Kind: "warmtenet"},
			{Kind: "electric_cooking"},
		},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 377500 {
		t.Fatalf("expected only warmtenet subsidy to apply, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateAirWaterHeatPumpFormulaByYear(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2026),
		Installations: []transport.RequestedInstallation{{
			Kind:                "heat_pump",
			HeatPumpType:        "air_water",
			HeatPumpEnergyLabel: "A+++",
			ThermalPowerKW:      floatPtr(4),
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 212500 {
		t.Fatalf("expected 2026 first air-water amount 212500 cents, got %d", resp.TotalAmountCents)
	}
}

func TestCalculateAdditionalAirWaterHeatPumpGetsReduced2026Amount(t *testing.T) {
	svc := New(testRepo{}, logger.New("development"))

	resp, err := svc.Calculate(context.Background(), uuid.New(), transport.ISDECalculationRequest{
		ExecutionYear: intPtr(2026),
		Installations: []transport.RequestedInstallation{{
			Kind:                "heat_pump",
			HeatPumpType:        "air_water",
			HeatPumpEnergyLabel: "A+++",
			ThermalPowerKW:      floatPtr(4),
			IsAdditionalUnit:    true,
		}},
	})
	if err != nil {
		t.Fatalf(errCalculateFmt, err)
	}
	if resp.TotalAmountCents != 90000 {
		t.Fatalf("expected reduced additional 2026 air-water amount 90000 cents, got %d", resp.TotalAmountCents)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func defaultTestMeasureConfigs(executionYear int) []repository.MeasureConfig {
	year := executionYear
	if year < 2024 {
		year = 2024
	}
	if year > 2026 {
		year = 2026
	}
	return []repository.MeasureConfig{
		{MeasureID: "roof", DisplayName: "Dakisolatie", Category: "insulation", QualifyingGroup: "insulation_roof_attic", MinM2: 20, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(3.5), RateMode: "standard", BaseRateCentsPerM2: map[int]int64{2024: 1500, 2025: 1625, 2026: 1625}[year], MaxM2: 200, MKIBonusCentsPerM2: 500},
		{MeasureID: "attic", DisplayName: "Zolder-/vlieringvloerisolatie", Category: "insulation", QualifyingGroup: "insulation_roof_attic", MinM2: 20, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(3.5), RateMode: "standard", BaseRateCentsPerM2: 400, MaxM2: map[int]float64{2024: 130, 2025: 200, 2026: 200}[year], MKIBonusCentsPerM2: 150},
		{MeasureID: "facade", DisplayName: "Gevelisolatie", Category: "insulation", QualifyingGroup: "insulation_facade", MinM2: 10, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(3.5), RateMode: "standard", BaseRateCentsPerM2: map[int]int64{2024: 1900, 2025: 2025, 2026: 2025}[year], MaxM2: 170, MKIBonusCentsPerM2: 600},
		{MeasureID: "cavity_wall", DisplayName: "Spouwmuurisolatie", Category: "insulation", QualifyingGroup: "insulation_cavity_wall", MinM2: 10, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(1.1), RateMode: "standard", BaseRateCentsPerM2: map[int]int64{2024: 400, 2025: 525, 2026: 525}[year], MaxM2: 170, MKIBonusCentsPerM2: 150},
		{MeasureID: "floor", DisplayName: "Vloerisolatie", Category: "insulation", QualifyingGroup: "insulation_floor_crawl_space", MinM2: 20, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(3.5), RateMode: "standard", BaseRateCentsPerM2: 550, MaxM2: 130, MKIBonusCentsPerM2: 200},
		{MeasureID: "crawl_space", DisplayName: "Bodemisolatie", Category: "insulation", QualifyingGroup: "insulation_floor_crawl_space", MinM2: 20, PerformanceRule: "rd_min", PerformanceThreshold: floatPtr(3.5), RateMode: "standard", BaseRateCentsPerM2: 300, MaxM2: 130, MKIBonusCentsPerM2: 100},
		{MeasureID: "hr_plus_plus", DisplayName: "HR++ glas", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(1.2), RateMode: "standard", BaseRateCentsPerM2: map[int]int64{2024: 2300, 2025: 2500, 2026: 2500}[year], MaxM2: 45},
		{MeasureID: "triple_glass", DisplayName: "Triple glas", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(0.7), RateMode: "upgraded_frame", BaseRateCentsPerM2: map[int]int64{2024: 2300, 2025: 2500, 2026: 2500}[year], UpgradedRateCentsPerM2: int64Ptr(map[int]int64{2024: 6550, 2025: 11100, 2026: 11100}[year]), MaxM2: 45, LegacyMaxFrameUValue: floatPtr(1.5)},
		{MeasureID: "vacuum_glass", DisplayName: "Vacuumglas", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(0.7), RateMode: "upgraded_frame", BaseRateCentsPerM2: map[int]int64{2024: 2300, 2025: 2500, 2026: 2500}[year], UpgradedRateCentsPerM2: int64Ptr(map[int]int64{2024: 6550, 2025: 11100, 2026: 11100}[year]), MaxM2: 45, LegacyMaxFrameUValue: floatPtr(1.5)},
		{MeasureID: "glass_panel_low", DisplayName: "Isolerend paneel", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(1.2), RateMode: "standard", BaseRateCentsPerM2: 1000, MaxM2: 45, RequiresPrimaryGlass: true},
		{MeasureID: "glass_panel_high", DisplayName: "Isolerend paneel hoogwaardig", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(0.7), RateMode: "standard", BaseRateCentsPerM2: 4500, MaxM2: 45, RequiresPrimaryGlass: true},
		{MeasureID: "insulated_door_low", DisplayName: "Isolerende deur", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(1.5), RateMode: "standard", BaseRateCentsPerM2: map[int]int64{2024: 2300, 2025: 2500, 2026: 2500}[year], MaxM2: 45, RequiresPrimaryGlass: true},
		{MeasureID: "insulated_door_high", DisplayName: "Isolerende deur hoogwaardig", Category: "glass", QualifyingGroup: "glass", MinM2: 0, PerformanceRule: "u_max", PerformanceThreshold: floatPtr(1.0), RateMode: "upgraded_frame", BaseRateCentsPerM2: map[int]int64{2024: 2300, 2025: 2500, 2026: 2500}[year], UpgradedRateCentsPerM2: int64Ptr(map[int]int64{2024: 6550, 2025: 11100, 2026: 11100}[year]), MaxM2: 45, RequiresPrimaryGlass: true, LegacyMaxFrameUValue: floatPtr(1.5)},
	}
}

func defaultTestProgramYearRule(executionYear int) repository.ProgramYearRule {
	year := executionYear
	if year < 2024 {
		year = 2024
	}
	if year > 2026 {
		year = 2026
	}
	return map[int]repository.ProgramYearRule{
		2024: {ExecutionYear: 2024, VentilationAmountCents: 0, WarmtenetAmountCents: 377500, ElectricCookingAmountCents: 40000, AirWaterStartAmountCents: 210000, AirWaterAmountPerKWCents: 15000, AirWaterAPlusPlusPlusBonusCents: 22500, AirWaterKWOffset: 1},
		2025: {ExecutionYear: 2025, VentilationAmountCents: 0, WarmtenetAmountCents: 377500, ElectricCookingAmountCents: 40000, AirWaterStartAmountCents: 125000, AirWaterAmountPerKWCents: 22500, AirWaterAPlusPlusPlusBonusCents: 20000, AirWaterKWOffset: 1},
		2026: {ExecutionYear: 2026, VentilationAmountCents: 40000, WarmtenetAmountCents: 377500, ElectricCookingAmountCents: 40000, AirWaterStartAmountCents: 102500, AirWaterAmountPerKWCents: 22500, AirWaterAPlusPlusPlusBonusCents: 20000, AirWaterKWOffset: 0},
	}[year]
}

func int64Ptr(v int64) *int64 {
	return &v
}
