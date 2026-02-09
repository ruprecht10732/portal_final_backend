package adapters

import (
	"context"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/ports"
	partnersvc "portal_final_backend/internal/partners/service"
)

// OfferSummaryGeneratorAdapter bridges leads summary generation to partners service.
type OfferSummaryGeneratorAdapter struct {
	generator ports.OfferSummaryGenerator
}

func NewOfferSummaryGeneratorAdapter(generator ports.OfferSummaryGenerator) *OfferSummaryGeneratorAdapter {
	return &OfferSummaryGeneratorAdapter{generator: generator}
}

func (a *OfferSummaryGeneratorAdapter) GenerateSummary(ctx context.Context, tenantID uuid.UUID, input partnersvc.OfferSummaryInput) (string, error) {
	if a.generator == nil {
		return "", nil
	}

	mapped := ports.OfferSummaryInput{
		LeadID:        input.LeadID,
		LeadServiceID: input.LeadServiceID,
		ServiceType:   input.ServiceType,
		Scope:         input.Scope,
		UrgencyLevel:  input.UrgencyLevel,
		Items:         make([]ports.OfferSummaryItem, 0, len(input.Items)),
	}

	for _, item := range input.Items {
		mapped.Items = append(mapped.Items, ports.OfferSummaryItem{
			Description: item.Description,
			Quantity:    item.Quantity,
		})
	}

	return a.generator.GenerateOfferSummary(ctx, tenantID, mapped)
}
