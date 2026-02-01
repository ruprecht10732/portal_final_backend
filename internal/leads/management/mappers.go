package management

import (
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
)

// ToLeadResponse converts a repository Lead to a transport LeadResponse.
func ToLeadResponse(lead repository.Lead) transport.LeadResponse {
	return transport.LeadResponse{
		ID:              lead.ID,
		AssignedAgentID: lead.AssignedAgentID,
		ViewedByID:      lead.ViewedByID,
		ViewedAt:        lead.ViewedAt,
		Source:          lead.Source,
		CreatedAt:       lead.CreatedAt,
		UpdatedAt:       lead.UpdatedAt,
		Services:        []transport.LeadServiceResponse{},
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
		ID:           svc.ID,
		ServiceType:  transport.ServiceType(svc.ServiceType),
		Status:       transport.LeadStatus(svc.Status),
		ConsumerNote: svc.ConsumerNote,
		CreatedAt:    svc.CreatedAt,
		UpdatedAt:    svc.UpdatedAt,
	}

	return resp
}
