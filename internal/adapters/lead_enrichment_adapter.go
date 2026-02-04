package adapters

import (
	"context"

	"portal_final_backend/internal/leadenrichment/service"
	"portal_final_backend/internal/leads/ports"
)

// LeadEnrichmentAdapter adapts the lead enrichment service for the RAC_leads domain.
type LeadEnrichmentAdapter struct {
	svc *service.Service
}

// NewLeadEnrichmentAdapter creates a new adapter that wraps the lead enrichment service.
// Returns nil if the service is nil (disabled).
func NewLeadEnrichmentAdapter(svc *service.Service) *LeadEnrichmentAdapter {
	if svc == nil {
		return nil
	}
	return &LeadEnrichmentAdapter{svc: svc}
}

// EnrichLead fetches enrichment data for a lead's postcode.
func (a *LeadEnrichmentAdapter) EnrichLead(ctx context.Context, postcode string) (*ports.LeadEnrichmentData, error) {
	if a == nil || a.svc == nil {
		return nil, nil
	}

	data, err := a.svc.GetByPostcode(ctx, postcode)
	if err != nil || data == nil {
		return nil, err
	}

	return &ports.LeadEnrichmentData{
		Source:                    data.Source,
		Postcode6:                 data.Postcode6,
		Postcode4:                 data.Postcode4,
		Buurtcode:                 data.Buurtcode,
		DataYear:                  data.DataYear,
		GemAardgasverbruik:        data.GemAardgasverbruik,
		GemElektriciteitsverbruik: data.GemElektriciteitsverbruik,
		HuishoudenGrootte:         data.HuishoudenGrootte,
		KoopwoningenPct:           data.KoopwoningenPct,
		BouwjaarVanaf2000Pct:      data.BouwjaarVanaf2000Pct,
		WOZWaarde:                 data.WOZWaarde,
		MediaanVermogenX1000:      data.MediaanVermogenX1000,
		GemInkomenHuishouden:      data.GemInkomenHuishouden,
		PctHoogInkomen:            data.PctHoogInkomen,
		PctLaagInkomen:            data.PctLaagInkomen,
		HuishoudensMetKinderenPct: data.HuishoudensMetKinderenPct,
		Stedelijkheid:             data.Stedelijkheid,
		Confidence:                data.Confidence,
	}, nil
}

// Compile-time check.
var _ ports.LeadEnricher = (*LeadEnrichmentAdapter)(nil)
