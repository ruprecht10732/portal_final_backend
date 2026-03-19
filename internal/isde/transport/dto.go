package transport

// ISDECalculationRequest represents a subsidy calculation request.
type ISDECalculationRequest struct {
	ExecutionYear                   *int                    `json:"executionYear,omitempty" validate:"omitempty,min=2024,max=2100"`
	PreviousSubsidiesWithin24Months bool                    `json:"previousSubsidiesWithin24Months"`
	HasExistingWarmtenetConnection  bool                    `json:"hasExistingWarmtenetConnection"`
	HasReceivedWarmtenetSubsidy     bool                    `json:"hasReceivedWarmtenetSubsidy"`
	Measures                        []RequestedMeasure      `json:"measures" validate:"omitempty,dive"`
	Installations                   []RequestedInstallation `json:"installations" validate:"omitempty,dive"`
}

// RequestedMeasure represents one requested insulation/glass measure.
type RequestedMeasure struct {
	MeasureID                string   `json:"measureId" validate:"required,min=1,max=80"`
	AreaM2                   float64  `json:"areaM2" validate:"min=0"`
	PerformanceValue         *float64 `json:"performanceValue,omitempty"`
	FramePerformanceValue    *float64 `json:"framePerformanceValue,omitempty"`
	HasMKIBonus              bool     `json:"hasMKIBonus"`
	FrameReplaced            bool     `json:"frameReplaced"`
	StackedWithPairedMeasure bool     `json:"stackedWithPairedMeasure"`
}

// RequestedInstallation represents one requested installation or flat-rate option.
type RequestedInstallation struct {
	Kind                string   `json:"kind,omitempty" validate:"omitempty,oneof=meldcode ventilation heat_pump warmtenet electric_cooking"`
	Meldcode            string   `json:"meldcode,omitempty" validate:"omitempty,min=1,max=40"`
	HeatPumpType        string   `json:"heatPumpType,omitempty" validate:"omitempty,oneof=air_water ground_water water_water heat_pump_boiler"`
	HeatPumpEnergyLabel string   `json:"heatPumpEnergyLabel,omitempty" validate:"omitempty,max=16"`
	ThermalPowerKW      *float64 `json:"thermalPowerKW,omitempty"`
	IsAdditionalUnit    bool     `json:"isAdditionalUnit"`
	IsSplitSystem       bool     `json:"isSplitSystem"`
	RefrigerantChargeKg *float64 `json:"refrigerantChargeKg,omitempty"`
	RefrigerantGWP      *float64 `json:"refrigerantGWP,omitempty"`
}

// ISDECalculationResponse mirrors the Afdrukoverzicht-style grouped output.
type ISDECalculationResponse struct {
	TotalAmountCents     int64          `json:"totalAmountCents"`
	IsDoubled            bool           `json:"isDoubled"`
	EligibleMeasureCount int            `json:"eligibleMeasureCount"`
	InsulationBreakdown  []ISDELineItem `json:"insulationBreakdown"`
	GlassBreakdown       []ISDELineItem `json:"glassBreakdown"`
	Installations        []ISDELineItem `json:"installations"`
	ValidationMessages   []string       `json:"validationMessages,omitempty"`
	UnknownMeasureIDs    []string       `json:"unknownMeasureIds,omitempty"`
	UnknownMeldcodes     []string       `json:"unknownMeldcodes,omitempty"`
}

// ISDELineItem is one row in the subsidy breakdown.
type ISDELineItem struct {
	Description string  `json:"description"`
	AreaM2      float64 `json:"areaM2,omitempty"`
	AmountCents int64   `json:"amountCents"`
}
