package adapters

import (
	"context"

	"portal_final_backend/internal/energylabel/service"
	"portal_final_backend/internal/leads/ports"
)

// EnergyLabelAdapter adapts the energylabel service for use by the leads domain.
// It implements the leads/ports.EnergyLabelEnricher interface.
type EnergyLabelAdapter struct {
	svc *service.Service
}

// NewEnergyLabelAdapter creates a new adapter that wraps the energylabel service.
// Returns nil if the service is nil (disabled).
func NewEnergyLabelAdapter(svc *service.Service) *EnergyLabelAdapter {
	if svc == nil {
		return nil
	}
	return &EnergyLabelAdapter{svc: svc}
}

// EnrichLead fetches energy label data for a lead's address.
// Translates the leads domain's EnrichLeadParams into energylabel service call
// and maps the response back to the leads domain's LeadEnergyData.
func (a *EnergyLabelAdapter) EnrichLead(ctx context.Context, params ports.EnrichLeadParams) (*ports.LeadEnergyData, error) {
	if a == nil || a.svc == nil {
		return nil, nil // Graceful degradation when disabled
	}

	label, err := a.svc.GetByAddress(ctx, params.Postcode, params.Huisnummer, params.Huisletter, params.Toevoeging, "")
	if err != nil {
		return nil, err
	}

	if label == nil {
		return nil, nil // No label found
	}

	// Map energylabel transport to leads port format
	return &ports.LeadEnergyData{
		Energieklasse:           label.Energieklasse,
		EnergieIndex:            label.EnergieIndex,
		Bouwjaar:                label.Bouwjaar,
		GeldigTot:               label.GeldigTot,
		Gebouwtype:              label.Gebouwtype,
		Registratiedatum:        label.Registratiedatum,
		PrimaireFossieleEnergie: label.PrimaireFossieleEnergie,
		BAGVerblijfsobjectID:    label.BAGVerblijfsobjectID,
	}, nil
}

// Compile-time check that EnergyLabelAdapter implements ports.EnergyLabelEnricher
var _ ports.EnergyLabelEnricher = (*EnergyLabelAdapter)(nil)
