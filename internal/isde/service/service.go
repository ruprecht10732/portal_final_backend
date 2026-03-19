package service

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"portal_final_backend/internal/isde/repository"
	"portal_final_backend/internal/isde/transport"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

const (
	historicalCategoryID             = "historical_within_24_months"
	measureCategoryRoofAttic         = "insulation_roof_attic"
	measureCategoryFloorCrawl        = "insulation_floor_crawl_space"
	measureCategoryFacade            = "insulation_facade"
	measureCategoryCavity            = "insulation_cavity_wall"
	measureCategoryGlass             = "glass"
	installationCategoryHeatPump     = "installation_heat_pump"
	installationCategorySolarBoiler  = "installation_solar_boiler"
	installationCategoryWarmtenet    = "installation_warmtenet"
	installationKindMeldcode         = "meldcode"
	installationKindVentilation      = "ventilation"
	installationKindHeatPump         = "heat_pump"
	installationKindWarmtenet        = "warmtenet"
	installationKindElectricCooking  = "electric_cooking"
	heatPumpTypeAirWater             = "air_water"
	heatPumpEnergyLabelAPlusPlus     = "A++"
	heatPumpEnergyLabelAPlusPlusPlus = "A+++"
	rateModeStandard                 = "standard"
	rateModeUpgradedFrame            = "upgraded_frame"
)

type performanceKind string

const (
	performanceKindNone performanceKind = "none"
	performanceKindRD   performanceKind = "rd_min"
	performanceKindU    performanceKind = "u_max"
)

type measureRule struct {
	ID                   string
	DisplayName          string
	Category             string
	QualifyingGroup      string
	PerformanceKind      performanceKind
	MinM2                float64
	Threshold            float64
	MaxM2                float64
	RateMode             string
	BaseRateCents        int64
	UpgradedRateCents    *int64
	MKIBonusCents        int64
	RequiresPrimaryGlass bool
	LegacyMaxFrameUValue *float64
}

type qualifiedMeasure struct {
	Requested     transport.RequestedMeasure
	Rule          measureRule
	QualifiedArea float64
}

// Service provides ISDE calculation logic.
type Service struct {
	repo repository.Repository
	log  *logger.Logger
}

// New creates a new ISDE service.
func New(repo repository.Repository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Calculate computes subsidy totals and grouped breakdowns.
func (s *Service) Calculate(ctx context.Context, tenantID uuid.UUID, req transport.ISDECalculationRequest) (transport.ISDECalculationResponse, error) {
	_ = tenantID // Reserved for future tenant-specific rule overrides.
	year := resolveCalculationYear(req.ExecutionYear)
	ruleYear := nearestRuleYear(year)

	measureRuleConfigs, err := s.repo.ListMeasureConfigsByIDsAndYear(ctx, collectMeasureIDs(req.Measures), ruleYear)
	if err != nil {
		return transport.ISDECalculationResponse{}, err
	}
	measureRuleByID := make(map[string]measureRule, len(measureRuleConfigs))
	for _, config := range measureRuleConfigs {
		measureRuleByID[normalizeMeasureID(config.MeasureID)] = mapMeasureConfig(config)
	}

	programRule, err := s.repo.GetProgramYearRule(ctx, ruleYear)
	if err != nil {
		return transport.ISDECalculationResponse{}, err
	}

	meldcodes := collectMeldcodes(req.Installations)
	installations, err := s.repo.ListInstallationMeldcodesByCodes(ctx, meldcodes)
	if err != nil {
		return transport.ISDECalculationResponse{}, err
	}
	installationByCode := make(map[string]repository.InstallationMeldcode, len(installations))
	for _, installation := range installations {
		installationByCode[normalizeMeldcode(installation.Meldcode)] = installation
	}

	resp := transport.ISDECalculationResponse{
		InsulationBreakdown: make([]transport.ISDELineItem, 0),
		GlassBreakdown:      make([]transport.ISDELineItem, 0),
		Installations:       make([]transport.ISDELineItem, 0),
	}

	qualifiedMeasures := buildQualifiedMeasures(req.Measures, measureRuleByID, year, &resp)
	qualifyingCategoryIDs := make(map[string]struct{}, len(qualifiedMeasures)+len(req.Installations)+1)
	if req.PreviousSubsidiesWithin24Months {
		qualifyingCategoryIDs[historicalCategoryID] = struct{}{}
	}

	for _, measure := range qualifiedMeasures {
		if measure.Rule.QualifyingGroup != "" {
			qualifyingCategoryIDs[measure.Rule.QualifyingGroup] = struct{}{}
		}
	}

	qualifyingInstallationCodes := make(map[string]struct{}, len(req.Installations))
	hasWarmtenetInRequest := false
	for _, requested := range req.Installations {
		kind := normalizeInstallationKind(requested.Kind, requested.Meldcode)
		if kind == installationKindWarmtenet {
			hasWarmtenetInRequest = true
			qualifyingCategoryIDs[installationCategoryWarmtenet] = struct{}{}
			continue
		}
		if kind == installationKindHeatPump {
			if qualifiesHeatPumpFormula(year, requested) {
				qualifyingCategoryIDs[installationCategoryHeatPump] = struct{}{}
			}
			continue
		}
		if kind == installationKindVentilation || kind == installationKindElectricCooking {
			continue
		}

		normalizedCode := normalizeMeldcode(requested.Meldcode)
		installation, ok := installationByCode[normalizedCode]
		if !ok {
			resp.UnknownMeldcodes = appendUnique(resp.UnknownMeldcodes, normalizedCode)
			continue
		}
		if _, alreadyAdded := qualifyingInstallationCodes[normalizedCode]; alreadyAdded {
			continue
		}
		qualifyingInstallationCodes[normalizedCode] = struct{}{}
		categoryID := normalizeInstallationCategoryID(installation.Category)
		if categoryID != "" {
			qualifyingCategoryIDs[categoryID] = struct{}{}
		}
	}

	resp.EligibleMeasureCount = len(qualifyingCategoryIDs)
	resp.IsDoubled = resp.EligibleMeasureCount >= 2

	for _, measure := range qualifiedMeasures {
		ratePerM2 := measureRateCents(measure.Rule, year, resp.IsDoubled, measure.Requested)
		description := measure.Rule.DisplayName
		baseAmountCents := areaTimesRateCents(measure.QualifiedArea, ratePerM2)
		mkiAmountCents := int64(0)
		if measure.Requested.HasMKIBonus {
			mkiAmountCents = areaTimesRateCents(measure.QualifiedArea, measure.Rule.MKIBonusCents)
			if mkiAmountCents > 0 {
				description += " (incl. MKI-bonus)"
			}
		}
		amountCents := baseAmountCents + mkiAmountCents
		line := transport.ISDELineItem{
			Description: description,
			AreaM2:      measure.QualifiedArea,
			AmountCents: amountCents,
		}

		switch strings.TrimSpace(strings.ToLower(measure.Rule.Category)) {
		case "glass":
			resp.GlassBreakdown = append(resp.GlassBreakdown, line)
		default:
			resp.InsulationBreakdown = append(resp.InsulationBreakdown, line)
		}
		resp.TotalAmountCents += amountCents
	}

	processedInstallationCodes := make(map[string]struct{}, len(req.Installations))
	for _, requested := range req.Installations {
		kind := normalizeInstallationKind(requested.Kind, requested.Meldcode)
		switch kind {
		case installationKindVentilation:
			if year < 2026 || len(qualifiedMeasures) == 0 {
				continue
			}
			line := transport.ISDELineItem{Description: "Ventilatie", AmountCents: programRule.VentilationAmountCents}
			resp.Installations = append(resp.Installations, line)
			resp.TotalAmountCents += line.AmountCents
			continue
		case installationKindWarmtenet:
			line := transport.ISDELineItem{Description: "Aansluiting warmtenet", AmountCents: programRule.WarmtenetAmountCents}
			resp.Installations = append(resp.Installations, line)
			resp.TotalAmountCents += line.AmountCents
			continue
		case installationKindElectricCooking:
			if hasWarmtenetInRequest || req.HasReceivedWarmtenetSubsidy || !req.HasExistingWarmtenetConnection {
				continue
			}
			line := transport.ISDELineItem{Description: "Elektrische kookvoorziening", AmountCents: programRule.ElectricCookingAmountCents}
			resp.Installations = append(resp.Installations, line)
			resp.TotalAmountCents += line.AmountCents
			continue
		case installationKindHeatPump:
			amountCents, description, ok := calculateHeatPumpFormula(programRule, year, requested)
			if !ok {
				continue
			}
			line := transport.ISDELineItem{Description: description, AmountCents: amountCents}
			resp.Installations = append(resp.Installations, line)
			resp.TotalAmountCents += line.AmountCents
			continue
		}

		normalizedCode := normalizeMeldcode(requested.Meldcode)
		if _, alreadyProcessed := processedInstallationCodes[normalizedCode]; alreadyProcessed {
			continue
		}
		installation, ok := installationByCode[normalizedCode]
		if !ok {
			continue
		}
		processedInstallationCodes[normalizedCode] = struct{}{}
		line := transport.ISDELineItem{
			Description: installationDescription(installation),
			AmountCents: installation.SubsidyAmountCents,
		}
		resp.Installations = append(resp.Installations, line)
		resp.TotalAmountCents += installation.SubsidyAmountCents
	}

	return resp, nil
}

func buildQualifiedMeasures(requestedMeasures []transport.RequestedMeasure, ruleByID map[string]measureRule, year int, resp *transport.ISDECalculationResponse) []qualifiedMeasure {
	qualified := make([]qualifiedMeasure, 0, len(requestedMeasures))
	for _, requested := range requestedMeasures {
		normalizedID := normalizeMeasureID(requested.MeasureID)
		rule, ok := ruleByID[normalizedID]
		if !ok {
			resp.UnknownMeasureIDs = appendUnique(resp.UnknownMeasureIDs, normalizedID)
			continue
		}
		if reason, ok := measureQualificationFailure(rule, requested); !ok {
			resp.ValidationMessages = appendUnique(resp.ValidationMessages, reason)
			continue
		}
		qualified = append(qualified, qualifiedMeasure{Requested: requested, Rule: rule, QualifiedArea: requested.AreaM2})
	}

	qualified = finalizeRoofAtticMeasures(year, qualified)
	qualified = finalizeFloorCrawlMeasures(year, qualified)
	qualified = finalizeGlassMeasures(year, qualified)
	qualified = finalizeIndividualMeasureCaps(qualified)
	return qualified
}

func finalizeRoofAtticMeasures(year int, measures []qualifiedMeasure) []qualifiedMeasure {
	return finalizeBucketMeasures(measures, year, measureCategoryRoofAttic, minAreaForBucket(measureCategoryRoofAttic), maxAreaForBucket(year, measureCategoryRoofAttic))
}

func finalizeFloorCrawlMeasures(year int, measures []qualifiedMeasure) []qualifiedMeasure {
	return finalizeBucketMeasures(measures, year, measureCategoryFloorCrawl, minAreaForBucket(measureCategoryFloorCrawl), maxAreaForBucket(year, measureCategoryFloorCrawl))
}

func finalizeBucketMeasures(measures []qualifiedMeasure, year int, bucket string, minArea float64, maxArea float64) []qualifiedMeasure {
	bucketMeasures := make([]qualifiedMeasure, 0)
	others := make([]qualifiedMeasure, 0, len(measures))
	for _, measure := range measures {
		if measure.Rule.QualifyingGroup == bucket {
			bucketMeasures = append(bucketMeasures, measure)
			continue
		}
		others = append(others, measure)
	}
	if len(bucketMeasures) == 0 {
		return measures
	}

	totalArea := sumAreas(bucketMeasures)
	if totalArea < minArea {
		return others
	}
	if hasStackedPair(bucketMeasures) {
		bestIdx := highestRateIndex(bucketMeasures, year)
		bucketMeasures = []qualifiedMeasure{bucketMeasures[bestIdx]}
		totalArea = sumAreas(bucketMeasures)
	}
	bucketMeasures = scaleAreasToCap(bucketMeasures, maxArea)
	return append(others, bucketMeasures...)
}

func finalizeGlassMeasures(year int, measures []qualifiedMeasure) []qualifiedMeasure {
	glassMeasures := make([]qualifiedMeasure, 0)
	others := make([]qualifiedMeasure, 0, len(measures))
	hasGlassBase := false
	for _, measure := range measures {
		if measure.Rule.QualifyingGroup != measureCategoryGlass {
			others = append(others, measure)
			continue
		}
		if isPrimaryGlassMeasure(measure.Rule.ID) {
			hasGlassBase = true
		}
		glassMeasures = append(glassMeasures, measure)
	}
	if len(glassMeasures) == 0 {
		return measures
	}
	filtered := make([]qualifiedMeasure, 0, len(glassMeasures))
	for _, measure := range glassMeasures {
		if measure.Rule.RequiresPrimaryGlass && !hasGlassBase {
			continue
		}
		filtered = append(filtered, measure)
	}
	totalArea := sumAreas(filtered)
	if totalArea < minGlassArea(year) {
		return others
	}
	filtered = scaleAreasToCap(filtered, 45)
	return append(others, filtered...)
}

func finalizeIndividualMeasureCaps(measures []qualifiedMeasure) []qualifiedMeasure {
	result := make([]qualifiedMeasure, 0, len(measures))
	for _, measure := range measures {
		if measure.Rule.QualifyingGroup == measureCategoryRoofAttic || measure.Rule.QualifyingGroup == measureCategoryFloorCrawl || measure.Rule.QualifyingGroup == measureCategoryGlass {
			result = append(result, measure)
			continue
		}
		capM2 := measure.Rule.MaxM2
		if capM2 > 0 && measure.QualifiedArea > capM2 {
			measure.QualifiedArea = capM2
		}
		result = append(result, measure)
	}
	return result
}

func installationDescription(installation repository.InstallationMeldcode) string {
	categoryLabel := "Installatie"
	switch strings.TrimSpace(strings.ToLower(installation.Category)) {
	case "heat_pump":
		categoryLabel = "Warmtepomp"
	case "solar_boiler":
		categoryLabel = "Zonneboiler"
	}
	return categoryLabel + " (" + installation.Meldcode + ")"
}

func areaTimesRateCents(areaM2 float64, centsPerM2 int64) int64 {
	if areaM2 <= 0 || centsPerM2 <= 0 {
		return 0
	}
	return int64(math.Round(areaM2 * float64(centsPerM2)))
}

func measureQualifies(rule measureRule, requested transport.RequestedMeasure) bool {
	_, ok := measureQualificationFailure(rule, requested)
	return ok
}

func measureQualificationFailure(rule measureRule, requested transport.RequestedMeasure) (string, bool) {
	if requested.AreaM2 < rule.MinM2 {
		return fmt.Sprintf("%s vereist minimaal %.0f m2.", rule.DisplayName, rule.MinM2), false
	}

	switch rule.PerformanceKind {
	case performanceKindNone:
		return "", true
	case performanceKindRD:
		if requested.PerformanceValue == nil {
			return fmt.Sprintf("%s vereist een Rd-waarde.", rule.DisplayName), false
		}
		if *requested.PerformanceValue >= rule.Threshold {
			return "", true
		}
		return fmt.Sprintf("%s vereist een Rd-waarde van minimaal %.1f.", rule.DisplayName, rule.Threshold), false
	case performanceKindU:
		if requested.PerformanceValue == nil {
			return fmt.Sprintf("%s vereist een U- of Ud-waarde.", rule.DisplayName), false
		}
		if *requested.PerformanceValue <= rule.Threshold {
			return "", true
		}
		return fmt.Sprintf("%s vereist een waarde van %.1f of lager.", rule.DisplayName, rule.Threshold), false
	default:
		return fmt.Sprintf("%s heeft ongeldige prestatieregels.", rule.DisplayName), false
	}
}

func collectMeasureIDs(measures []transport.RequestedMeasure) []string {
	ids := make([]string, 0, len(measures))
	seen := make(map[string]struct{}, len(measures))
	for _, measure := range measures {
		normalizedID := normalizeMeasureID(measure.MeasureID)
		if normalizedID == "" {
			continue
		}
		if _, ok := seen[normalizedID]; ok {
			continue
		}
		seen[normalizedID] = struct{}{}
		ids = append(ids, normalizedID)
	}
	return ids
}

func collectMeldcodes(installations []transport.RequestedInstallation) []string {
	codes := make([]string, 0, len(installations))
	seen := make(map[string]struct{}, len(installations))
	for _, installation := range installations {
		if normalizeInstallationKind(installation.Kind, installation.Meldcode) != installationKindMeldcode {
			continue
		}
		normalizedCode := normalizeMeldcode(installation.Meldcode)
		if normalizedCode == "" {
			continue
		}
		if _, ok := seen[normalizedCode]; ok {
			continue
		}
		seen[normalizedCode] = struct{}{}
		codes = append(codes, normalizedCode)
	}
	return codes
}

func normalizeMeasureID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeMeldcode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeInstallationKind(kind, meldcode string) string {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	if normalized != "" {
		return normalized
	}
	if strings.TrimSpace(meldcode) != "" {
		return installationKindMeldcode
	}
	return installationKindMeldcode
}

func normalizeInstallationCategoryID(category string) string {
	normalized := strings.ToLower(strings.TrimSpace(category))
	if normalized == "" {
		return ""
	}
	return "installation_" + normalized
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func resolveCalculationYear(requestedYear *int) int {
	if requestedYear == nil || *requestedYear < 2024 {
		return time.Now().Year()
	}
	return *requestedYear
}

func nearestRuleYear(year int) int {
	switch {
	case year <= 2024:
		return 2024
	case year == 2025:
		return 2025
	default:
		return 2026
	}
}

func minAreaForBucket(bucket string) float64 {
	switch bucket {
	case measureCategoryRoofAttic, measureCategoryFloorCrawl:
		return 20
	default:
		return 0
	}
}

func maxAreaForBucket(year int, bucket string) float64 {
	switch bucket {
	case measureCategoryRoofAttic:
		if year <= 2024 {
			return 0
		}
		return 200
	case measureCategoryFloorCrawl:
		return 130
	default:
		return 0
	}
}

func minGlassArea(year int) float64 {
	if year <= 2024 {
		return 8
	}
	return 3
}

func mapMeasureConfig(config repository.MeasureConfig) measureRule {
	rule := measureRule{
		ID:                   normalizeMeasureID(config.MeasureID),
		DisplayName:          strings.TrimSpace(config.DisplayName),
		Category:             strings.TrimSpace(config.Category),
		QualifyingGroup:      strings.TrimSpace(config.QualifyingGroup),
		MinM2:                config.MinM2,
		Threshold:            derefFloat64(config.PerformanceThreshold),
		MaxM2:                config.MaxM2,
		RateMode:             strings.TrimSpace(config.RateMode),
		BaseRateCents:        config.BaseRateCentsPerM2,
		UpgradedRateCents:    config.UpgradedRateCentsPerM2,
		MKIBonusCents:        config.MKIBonusCentsPerM2,
		RequiresPrimaryGlass: config.RequiresPrimaryGlass,
		LegacyMaxFrameUValue: config.LegacyMaxFrameUValue,
	}
	switch strings.ToLower(strings.TrimSpace(config.PerformanceRule)) {
	case string(performanceKindRD):
		rule.PerformanceKind = performanceKindRD
	case string(performanceKindU):
		rule.PerformanceKind = performanceKindU
	default:
		rule.PerformanceKind = performanceKindNone
	}
	return rule
}

func measureRateCents(rule measureRule, year int, doubled bool, requested transport.RequestedMeasure) int64 {
	rate := rule.BaseRateCents
	if rule.RateMode == rateModeUpgradedFrame && rule.UpgradedRateCents != nil && requested.FrameReplaced {
		useUpgraded := true
		if year <= 2025 && rule.LegacyMaxFrameUValue != nil && requested.FramePerformanceValue != nil && *requested.FramePerformanceValue > *rule.LegacyMaxFrameUValue {
			useUpgraded = false
		}
		if useUpgraded {
			rate = *rule.UpgradedRateCents
		}
	}
	if doubled {
		return rate * 2
	}
	return rate
}

func sumAreas(measures []qualifiedMeasure) float64 {
	total := 0.0
	for _, measure := range measures {
		total += measure.QualifiedArea
	}
	return total
}

func scaleAreasToCap(measures []qualifiedMeasure, capM2 float64) []qualifiedMeasure {
	if capM2 <= 0 {
		return measures
	}
	totalArea := sumAreas(measures)
	if totalArea <= capM2 || totalArea <= 0 {
		return measures
	}
	ratio := capM2 / totalArea
	result := make([]qualifiedMeasure, 0, len(measures))
	for _, measure := range measures {
		measure.QualifiedArea = measure.QualifiedArea * ratio
		result = append(result, measure)
	}
	return result
}

func highestRateIndex(measures []qualifiedMeasure, year int) int {
	bestIdx := 0
	bestRate := int64(0)
	for idx, measure := range measures {
		rate := measureRateCents(measure.Rule, year, false, measure.Requested)
		if rate > bestRate {
			bestRate = rate
			bestIdx = idx
		}
	}
	return bestIdx
}

func hasStackedPair(measures []qualifiedMeasure) bool {
	if len(measures) < 2 {
		return false
	}
	for _, measure := range measures {
		if measure.Requested.StackedWithPairedMeasure {
			return true
		}
	}
	return false
}

func isPrimaryGlassMeasure(measureID string) bool {
	switch measureID {
	case "hr_plus_plus", "triple_glass", "vacuum_glass":
		return true
	default:
		return false
	}
}

func qualifiesHeatPumpFormula(year int, requested transport.RequestedInstallation) bool {
	if normalizeHeatPumpType(requested.HeatPumpType) != heatPumpTypeAirWater {
		return false
	}
	if requested.ThermalPowerKW == nil || *requested.ThermalPowerKW <= 0 {
		return false
	}
	label := normalizeHeatPumpEnergyLabel(requested.HeatPumpEnergyLabel)
	if year >= 2024 && label != heatPumpEnergyLabelAPlusPlus && label != heatPumpEnergyLabelAPlusPlusPlus {
		return false
	}
	if year >= 2026 && requested.IsSplitSystem && requested.RefrigerantChargeKg != nil && requested.RefrigerantGWP != nil && *requested.RefrigerantChargeKg < 3 && *requested.RefrigerantGWP > 750 {
		return false
	}
	return true
}

func calculateHeatPumpFormula(programRule repository.ProgramYearRule, year int, requested transport.RequestedInstallation) (int64, string, bool) {
	if !qualifiesHeatPumpFormula(year, requested) {
		return 0, "", false
	}
	label := normalizeHeatPumpEnergyLabel(requested.HeatPumpEnergyLabel)
	powerKW := *requested.ThermalPowerKW
	amountCents := int64(math.Round(math.Max(powerKW-programRule.AirWaterKWOffset, 0) * float64(programRule.AirWaterAmountPerKWCents)))
	if !requested.IsAdditionalUnit {
		amountCents += programRule.AirWaterStartAmountCents
		if label == heatPumpEnergyLabelAPlusPlusPlus {
			amountCents += programRule.AirWaterAPlusPlusPlusBonusCents
		}
	}
	description := fmt.Sprintf("Lucht-water warmtepomp %s (%.2f kW)", label, powerKW)
	if requested.IsAdditionalUnit && year >= 2026 {
		description = "Extra lucht-water warmtepomp"
	}
	return amountCents, description, true
}

func derefFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func normalizeHeatPumpType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHeatPumpEnergyLabel(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
