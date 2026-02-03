package ports

import "context"

// LeadEnrichmentData contains enrichment data relevant for leads.
type LeadEnrichmentData struct {
	Source    string
	Postcode6 string
	Postcode4 string
	Buurtcode string
	DataYear  *int // Year of PC4/PC6 data (e.g., 2022, 2023, 2024)

	// Energy
	GemAardgasverbruik        *float64
	GemElektriciteitsverbruik *float64

	// Housing
	HuishoudenGrootte    *float64
	KoopwoningenPct      *float64
	BouwjaarVanaf2000Pct *float64
	WOZWaarde            *float64

	// Income
	MediaanVermogenX1000 *float64
	GemInkomenHuishouden *float64
	PctHoogInkomen       *float64
	PctLaagInkomen       *float64

	// Demographics
	HuishoudensMetKinderenPct *float64
	Stedelijkheid             *int

	Confidence *float64
}

// LeadEnricher enriches a lead with PDOK/CBS data.
type LeadEnricher interface {
	EnrichLead(ctx context.Context, postcode string) (*LeadEnrichmentData, error)
}
