package ports

import (
	"context"
	"time"
)

// LeadEnergyData contains the energy label data relevant for RAC_leads.
// This is defined by the RAC_leads domain - only the fields RAC_leads cares about.
type LeadEnergyData struct {
	Energieklasse           string     `json:"energieklasse"`                     // Energy label class (A+++, A++, A+, A, B, C, D, E, F, G)
	EnergieIndex            *float64   `json:"energieIndex,omitempty"`            // Energy index value
	Bouwjaar                int        `json:"bouwjaar,omitempty"`                // Construction year
	GeldigTot               *time.Time `json:"geldigTot,omitempty"`               // Label validity end date
	Gebouwtype              string     `json:"gebouwtype,omitempty"`              // Building type
	Registratiedatum        *time.Time `json:"registratiedatum,omitempty"`        // When the label was registered
	PrimaireFossieleEnergie *float64   `json:"primaireFossieleEnergie,omitempty"` // Primary fossil energy use (kWh/m2·jaar)
	BAGVerblijfsobjectID    string     `json:"bagVerblijfsobjectId,omitempty"`    // BAG ID for future lookups
}

// EnrichLeadParams contains the address parameters for energy label enrichment.
type EnrichLeadParams struct {
	Postcode   string
	Huisnummer string
	Huisletter string
	Toevoeging string
}

// EnergyLabelEnricher is the interface that the RAC_leads domain uses to enrich RAC_leads
// with energy label data. The implementation is provided by the composition root
// and wraps the energylabel service.
type EnergyLabelEnricher interface {
	// EnrichLead fetches energy label data for a lead's address.
	// Returns nil if no label is found (not an error).
	// Returns error only if the lookup fails unexpectedly.
	EnrichLead(ctx context.Context, params EnrichLeadParams) (*LeadEnergyData, error)
}
