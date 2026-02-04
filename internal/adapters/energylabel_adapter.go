package adapters

import (
	"context"
	"strings"
	"unicode"

	"portal_final_backend/internal/energylabel/service"
	"portal_final_backend/internal/leads/ports"
)

// EnergyLabelAdapter adapts the energylabel service for use by the RAC_leads domain.
// It implements the RAC_leads/ports.EnergyLabelEnricher interface.
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
// Translates the RAC_leads domain's EnrichLeadParams into energylabel service call
// and maps the response back to the RAC_leads domain's LeadEnergyData.
func (a *EnergyLabelAdapter) EnrichLead(ctx context.Context, params ports.EnrichLeadParams) (*ports.LeadEnergyData, error) {
	if a == nil || a.svc == nil {
		return nil, nil // Graceful degradation when disabled
	}

	normalized, ok := normalizeAddressParams(params)
	if !ok {
		return nil, nil
	}

	label, err := a.svc.GetByAddress(ctx, normalized.Postcode, normalized.Huisnummer, normalized.Huisletter, normalized.Toevoeging, "")
	if err != nil {
		return nil, err
	}

	if label == nil {
		return nil, nil // No label found
	}

	// Map energylabel transport to RAC_leads port format
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

func normalizeAddressParams(params ports.EnrichLeadParams) (ports.EnrichLeadParams, bool) {
	postcode := sanitizePostcode(params.Postcode)
	if postcode == "" {
		return ports.EnrichLeadParams{}, false
	}

	huisnummer, huisletter, toevoeging := splitHouseComponents(params.Huisnummer)

	if huisnummer == "" {
		return ports.EnrichLeadParams{}, false
	}

	// Preserve explicit letter or addition provided by caller when available.
	if params.Huisletter != "" {
		huisletter = params.Huisletter
	}
	if params.Toevoeging != "" {
		toevoeging = params.Toevoeging
	}

	return ports.EnrichLeadParams{
		Postcode:   postcode,
		Huisnummer: huisnummer,
		Huisletter: huisletter,
		Toevoeging: toevoeging,
	}, true
}

func sanitizePostcode(value string) string {
	upper := strings.ToUpper(strings.ReplaceAll(value, " ", ""))
	upper = strings.ReplaceAll(upper, "-", "")
	return strings.TrimSpace(upper)
}

func splitHouseComponents(raw string) (number string, letter string, addition string) {
	cleaned := strings.TrimSpace(strings.ToUpper(raw))
	if cleaned == "" {
		return "", "", ""
	}

	// Extract leading digits as house number
	var digitsBuilder strings.Builder
	var idx int
	for idx < len(cleaned) {
		r := rune(cleaned[idx])
		if !unicode.IsDigit(r) {
			break
		}
		digitsBuilder.WriteRune(r)
		idx++
	}

	number = digitsBuilder.String()
	if number == "" {
		return "", "", ""
	}

	remainder := strings.TrimSpace(cleaned[idx:])
	if remainder == "" {
		return number, "", ""
	}

	// Single trailing letter (e.g., 46B)
	if len(remainder) == 1 && unicode.IsLetter(rune(remainder[0])) {
		return number, remainder, ""
	}

	// Remainder may contain separators (e.g., 46-2, 46 A1)
	remainder = strings.TrimLeft(remainder, "- /")
	if remainder == "" {
		return number, "", ""
	}

	if unicode.IsLetter(rune(remainder[0])) && len(remainder) > 1 {
		letter = string(remainder[0])
		addition = strings.TrimLeft(remainder[1:], "- /")
		return number, letter, addition
	}

	if unicode.IsLetter(rune(remainder[0])) {
		return number, string(remainder[0]), ""
	}

	return number, "", remainder
}
