package management

import (
	"encoding/json"
	"strings"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
)

func energyLabelFromLead(lead repository.Lead) *transport.EnergyLabelResponse {
	if lead.EnergyClass == nil {
		return nil
	}

	resp := &transport.EnergyLabelResponse{
		Energieklasse:           *lead.EnergyClass,
		EnergieIndex:            lead.EnergyIndex,
		GeldigTot:               lead.EnergyLabelValidUntil,
		Registratiedatum:        lead.EnergyLabelRegisteredAt,
		PrimaireFossieleEnergie: lead.EnergyPrimairFossiel,
	}

	if lead.EnergyBouwjaar != nil {
		resp.Bouwjaar = *lead.EnergyBouwjaar
	}

	if lead.EnergyGebouwtype != nil {
		resp.Gebouwtype = *lead.EnergyGebouwtype
	}

	return resp
}

func leadEnrichmentFromLead(lead repository.Lead) *transport.LeadEnrichmentResponse {
	if lead.LeadEnrichmentSource == nil && lead.LeadEnrichmentFetchedAt == nil {
		return nil
	}

	return &transport.LeadEnrichmentResponse{
		Source:                    lead.LeadEnrichmentSource,
		Postcode6:                 lead.LeadEnrichmentPostcode6,
		Postcode4:                 lead.LeadEnrichmentPostcode4,
		Buurtcode:                 lead.LeadEnrichmentBuurtcode,
		DataYear:                  lead.LeadEnrichmentDataYear,
		GemAardgasverbruik:        lead.LeadEnrichmentGemAardgasverbruik,
		GemElektriciteitsverbruik: lead.LeadEnrichmentGemElektriciteitsverbruik,
		HuishoudenGrootte:         lead.LeadEnrichmentHuishoudenGrootte,
		KoopwoningenPct:           lead.LeadEnrichmentKoopwoningenPct,
		BouwjaarVanaf2000Pct:      lead.LeadEnrichmentBouwjaarVanaf2000Pct,
		WOZWaarde:                 lead.LeadEnrichmentWOZWaarde,
		MediaanVermogenX1000:      lead.LeadEnrichmentMediaanVermogenX1000,
		GemInkomen:                lead.LeadEnrichmentGemInkomen,
		PctHoogInkomen:            lead.LeadEnrichmentPctHoogInkomen,
		PctLaagInkomen:            lead.LeadEnrichmentPctLaagInkomen,
		HuishoudensMetKinderenPct: lead.LeadEnrichmentHuishoudensMetKinderenPct,
		Stedelijkheid:             lead.LeadEnrichmentStedelijkheid,
		Confidence:                lead.LeadEnrichmentConfidence,
		FetchedAt:                 lead.LeadEnrichmentFetchedAt,
	}
}

func leadScoreFromLead(lead repository.Lead) *transport.LeadScoreResponse {
	if lead.LeadScore == nil && lead.LeadScorePreAI == nil {
		return nil
	}

	var factors json.RawMessage
	if len(lead.LeadScoreFactors) > 0 {
		factors = json.RawMessage(lead.LeadScoreFactors)
	}

	return &transport.LeadScoreResponse{
		Score:     lead.LeadScore,
		PreAI:     lead.LeadScorePreAI,
		Factors:   factors,
		Version:   lead.LeadScoreVersion,
		UpdatedAt: lead.LeadScoreUpdatedAt,
	}
}

// ToLeadResponse converts a repository Lead to a transport LeadResponse.
func ToLeadResponse(lead repository.Lead) transport.LeadResponse {
	return transport.LeadResponse{
		ID:              lead.ID,
		AssignedAgentID: lead.AssignedAgentID,
		ViewedByID:      lead.ViewedByID,
		ViewedAt:        lead.ViewedAt,
		Source:          lead.Source,
		WhatsAppOptedIn: lead.WhatsAppOptedIn,
		CreatedAt:       lead.CreatedAt,
		UpdatedAt:       lead.UpdatedAt,
		Services:        []transport.LeadServiceResponse{},
		EnergyLabel:     energyLabelFromLead(lead),
		LeadEnrichment:  leadEnrichmentFromLead(lead),
		LeadScore:       leadScoreFromLead(lead),
		Consumer: transport.ConsumerResponse{
			FirstName: lead.ConsumerFirstName,
			LastName:  lead.ConsumerLastName,
			Phone:     lead.ConsumerPhone,
			Email:     lead.ConsumerEmail,
			Role:      transport.ConsumerRole(lead.ConsumerRole),
		},
		Address: transport.AddressResponse{
			Street:      lead.AddressStreet,
			HouseNumber: lead.AddressHouseNumber,
			ZipCode:     lead.AddressZipCode,
			City:        lead.AddressCity,
			Latitude:    lead.Latitude,
			Longitude:   lead.Longitude,
		},
	}
}

// ToLeadResponseWithServices converts a repository Lead with services to a transport LeadResponse.
func ToLeadResponseWithServices(lead repository.Lead, services []repository.LeadService) transport.LeadResponse {
	resp := ToLeadResponse(lead)

	resp.Services = make([]transport.LeadServiceResponse, len(services))
	for i, svc := range services {
		resp.Services[i] = ToLeadServiceResponse(svc)
	}

	// Set current service (first non-terminal or first if all terminal)
	if len(services) > 0 {
		for _, svc := range services {
			if svc.Status != "Closed" && svc.Status != "Bad_Lead" && svc.Status != "Surveyed" {
				svcResp := ToLeadServiceResponse(svc)
				resp.CurrentService = &svcResp
				status := transport.LeadStatus(svc.Status)
				resp.AggregateStatus = &status
				break
			}
		}
		if resp.CurrentService == nil {
			svcResp := ToLeadServiceResponse(services[0])
			resp.CurrentService = &svcResp
			status := transport.LeadStatus(services[0].Status)
			resp.AggregateStatus = &status
		}
	}

	return resp
}

// ToLeadServiceResponse converts a repository LeadService to a transport LeadServiceResponse.
func ToLeadServiceResponse(svc repository.LeadService) transport.LeadServiceResponse {
	resp := transport.LeadServiceResponse{
		ID:            svc.ID,
		ServiceType:   transport.ServiceType(svc.ServiceType),
		Status:        transport.LeadStatus(svc.Status),
		PipelineStage: transport.PipelineStage(svc.PipelineStage),
		Preferences:   leadPreferencesFromService(svc),
		ConsumerNote:  svc.ConsumerNote,
		CreatedAt:     svc.CreatedAt,
		UpdatedAt:     svc.UpdatedAt,
	}

	return resp
}

func leadPreferencesFromService(svc repository.LeadService) *transport.LeadPreferencesResponse {
	if len(svc.CustomerPreferences) == 0 {
		return nil
	}

	var raw struct {
		Budget       string `json:"budget"`
		Timeframe    string `json:"timeframe"`
		Availability string `json:"availability"`
		ExtraNotes   string `json:"extraNotes"`
	}
	if err := json.Unmarshal(svc.CustomerPreferences, &raw); err != nil {
		return nil
	}

	prefs := transport.LeadPreferencesResponse{
		Budget:       normalizePreference(raw.Budget),
		Timeframe:    normalizePreference(raw.Timeframe),
		Availability: normalizePreference(raw.Availability),
		ExtraNotes:   normalizePreference(raw.ExtraNotes),
	}

	if prefs.Budget == nil && prefs.Timeframe == nil && prefs.Availability == nil && prefs.ExtraNotes == nil {
		return nil
	}

	return &prefs
}

func normalizePreference(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
