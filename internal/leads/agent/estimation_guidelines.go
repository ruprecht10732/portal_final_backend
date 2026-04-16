package agent

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const guidelinesLoadTimeout = 10 * time.Second

const defaultCustomerCommunicationGuideline = "Customer communication note: Avoid technical jargon in customer-facing clarification messages. Translate trade terms into simple consumer language with concrete examples of what to measure or photograph."

func fetchServiceTypeEstimationGuidelines(ctx context.Context, repo repository.LeadsRepository, tenantID uuid.UUID, serviceType string) string {
	ioCtx, ioCancel := detachedTimeout(ctx, guidelinesLoadTimeout)
	defer ioCancel()
	serviceTypes, err := repo.ListActiveServiceTypes(ioCtx, tenantID)
	if err != nil {
		log.Printf("guidelines: estimation guidelines load failed tenant=%s serviceType=%s: %v", tenantID, serviceType, err)
		return defaultCustomerCommunicationGuideline
	}
	for _, serviceDefinition := range serviceTypes {
		if serviceDefinition.Name != serviceType || serviceDefinition.EstimationGuidelines == nil {
			continue
		}
		guidelines := strings.TrimSpace(*serviceDefinition.EstimationGuidelines)
		if guidelines == "" {
			log.Printf("guidelines: estimation guidelines empty tenant=%s serviceType=%s", tenantID, serviceType)
			return defaultCustomerCommunicationGuideline
		}
		log.Printf("guidelines: estimation guidelines loaded tenant=%s serviceType=%s len=%d", tenantID, serviceType, len(guidelines))
		return guidelines + "\n\n" + defaultCustomerCommunicationGuideline
	}
	log.Printf("guidelines: estimation guidelines not found tenant=%s serviceType=%s", tenantID, serviceType)
	return defaultCustomerCommunicationGuideline
}
