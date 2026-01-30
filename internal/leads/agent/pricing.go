package agent

import "strings"

// getServicePricing returns typical pricing for common services
func getServicePricing(category, serviceType, urgency string) GetPricingOutput {
	// Base pricing by category (in euros)
	basePricing := map[string]map[string]GetPricingOutput{
		"plumbing": {
			"leaky_faucet": {
				PriceRangeLow:    75,
				PriceRangeHigh:   150,
				TypicalDuration:  "30-60 minuten",
				IncludedServices: []string{"Diagnose", "Reparatie", "Materiaal (standaard)"},
				Notes:            "Exclusief eventuele nieuwe kraan",
			},
			"clogged_drain": {
				PriceRangeLow:    95,
				PriceRangeHigh:   200,
				TypicalDuration:  "30-90 minuten",
				IncludedServices: []string{"Ontstopping", "Inspectie", "Advies preventie"},
				Notes:            "Camera-inspectie extra indien nodig",
			},
			"boiler_repair": {
				PriceRangeLow:    150,
				PriceRangeHigh:   350,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Diagnose", "Reparatie", "Veiligheidscheck"},
				Notes:            "Onderdelen worden apart berekend",
			},
			"default": {
				PriceRangeLow:    65,
				PriceRangeHigh:   250,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Voorrijkosten", "Eerste uur arbeid"},
				Notes:            "Exacte prijs na inspectie ter plaatse",
			},
		},
		"hvac": {
			"heating_repair": {
				PriceRangeLow:    125,
				PriceRangeHigh:   300,
				TypicalDuration:  "1-3 uur",
				IncludedServices: []string{"Diagnose", "Reparatie", "Veiligheidstest"},
				Notes:            "CV-ketel onderdelen apart",
			},
			"ac_service": {
				PriceRangeLow:    150,
				PriceRangeHigh:   250,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Controle", "Bijvullen koelmiddel", "Filter schoonmaken"},
				Notes:            "Jaarlijks onderhoud aanbevolen",
			},
			"heat_pump_install": {
				PriceRangeLow:    3500,
				PriceRangeHigh:   8000,
				TypicalDuration:  "1-2 dagen",
				IncludedServices: []string{"Installatie", "Aansluiting", "Inregelen", "Garantie"},
				Notes:            "Subsidie mogelijk (ISDE)",
			},
			"default": {
				PriceRangeLow:    95,
				PriceRangeHigh:   200,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Voorrijkosten", "Diagnose"},
				Notes:            "Reparatiekosten worden vooraf besproken",
			},
		},
		"electrical": {
			"outlet_install": {
				PriceRangeLow:    85,
				PriceRangeHigh:   150,
				TypicalDuration:  "30-60 minuten",
				IncludedServices: []string{"Installatie", "Materiaal", "Veiligheidstest"},
				Notes:            "Per stopcontact",
			},
			"fuse_box_upgrade": {
				PriceRangeLow:    500,
				PriceRangeHigh:   1200,
				TypicalDuration:  "4-8 uur",
				IncludedServices: []string{"Nieuwe groepenkast", "Aarding", "Certificering"},
				Notes:            "Verplichte NEN1010 keuring",
			},
			"ev_charger": {
				PriceRangeLow:    750,
				PriceRangeHigh:   1500,
				TypicalDuration:  "3-5 uur",
				IncludedServices: []string{"Laadpaal", "Installatie", "Keuring"},
				Notes:            "Afhankelijk van afstand tot meterkast",
			},
			"default": {
				PriceRangeLow:    75,
				PriceRangeHigh:   175,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Voorrijkosten", "Klein materiaal"},
				Notes:            "Grotere klussen op offerte",
			},
		},
		"carpentry": {
			"door_repair": {
				PriceRangeLow:    95,
				PriceRangeHigh:   200,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Reparatie", "Bijstellen", "Hang- en sluitwerk"},
				Notes:            "Nieuwe deur indien nodig: offerte",
			},
			"floor_install": {
				PriceRangeLow:    25,
				PriceRangeHigh:   45,
				TypicalDuration:  "1-2 dagen (gemiddelde kamer)",
				IncludedServices: []string{"Leggen", "Ondervloer", "Plinten"},
				Notes:            "Prijs per m², exclusief materiaal",
			},
			"kitchen_install": {
				PriceRangeLow:    500,
				PriceRangeHigh:   1500,
				TypicalDuration:  "1-3 dagen",
				IncludedServices: []string{"Montage kasten", "Werkblad", "Afwerking"},
				Notes:            "Exclusief aansluitingen (apart door specialist)",
			},
			"default": {
				PriceRangeLow:    50,
				PriceRangeHigh:   150,
				TypicalDuration:  "1-4 uur",
				IncludedServices: []string{"Arbeidsloon"},
				Notes:            "Materiaal wordt apart berekend",
			},
		},
		"general": {
			"default": {
				PriceRangeLow:    45,
				PriceRangeHigh:   95,
				TypicalDuration:  "1-2 uur",
				IncludedServices: []string{"Voorrijkosten", "Eerste uur"},
				Notes:            "Klusjesmannen, all-round",
			},
		},
	}

	// Urgency multipliers
	urgencyMultiplier := 1.0
	urgencyNote := ""
	switch urgency {
	case "same_day":
		urgencyMultiplier = 1.25
		urgencyNote = " (toeslag dezelfde dag service)"
	case "emergency":
		urgencyMultiplier = 1.5
		urgencyNote = " (spoedtarief 24/7)"
	}

	// Get pricing for category
	categoryPricing, ok := basePricing[strings.ToLower(category)]
	if !ok {
		categoryPricing = basePricing["general"]
	}

	// Get pricing for specific service type
	pricing, ok := categoryPricing[strings.ToLower(serviceType)]
	if !ok {
		pricing = categoryPricing["default"]
	}

	// Apply urgency multiplier
	pricing.PriceRangeLow = int(float64(pricing.PriceRangeLow) * urgencyMultiplier)
	pricing.PriceRangeHigh = int(float64(pricing.PriceRangeHigh) * urgencyMultiplier)
	pricing.Notes += urgencyNote

	return pricing
}
