package service

import (
	"context"
	"strings"

	"portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

func (s *Service) GetPricingIntelligenceReport(ctx context.Context, tenantID uuid.UUID, serviceType string, postcodePrefix string) (*repository.PricingIntelligenceReport, error) {
	return s.repo.GetPricingIntelligenceReport(ctx, tenantID, strings.TrimSpace(serviceType), strings.TrimSpace(postcodePrefix))
}