package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const defaultCustomerCommunicationGuideline = "Customer communication note: Avoid technical jargon in customer-facing clarification messages. Translate trade terms into simple consumer language with concrete examples of what to measure or photograph."

func fetchServiceTypeEstimationGuidelines(ctx context.Context, repo repository.LeadsRepository, tenantID uuid.UUID, serviceType string) string {
	serviceTypes, err := repo.ListActiveServiceTypes(ctx, tenantID)
	if err != nil {
		return defaultCustomerCommunicationGuideline
	}
	for _, serviceDefinition := range serviceTypes {
		if serviceDefinition.Name != serviceType || serviceDefinition.EstimationGuidelines == nil {
			continue
		}
		guidelines := strings.TrimSpace(*serviceDefinition.EstimationGuidelines)
		if guidelines == "" {
			return defaultCustomerCommunicationGuideline
		}
		return guidelines + "\n\n" + defaultCustomerCommunicationGuideline
	}
	return defaultCustomerCommunicationGuideline
}
