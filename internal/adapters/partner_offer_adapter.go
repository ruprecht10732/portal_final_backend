package adapters

import (
	"context"
	"time"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/partners/service"
	"portal_final_backend/internal/partners/transport"

	"github.com/google/uuid"
)

type PartnerOfferAdapter struct {
	service *service.Service
}

func NewPartnerOfferAdapter(s *service.Service) *PartnerOfferAdapter {
	return &PartnerOfferAdapter{service: s}
}

func (a *PartnerOfferAdapter) CreateOfferFromQuote(ctx context.Context, tenantID uuid.UUID, req ports.CreateOfferFromQuoteParams) (*ports.CreateOfferResult, error) {
	transportReq := transport.CreateOfferFromQuoteRequest{
		PartnerID:       req.PartnerID,
		QuoteID:         req.QuoteID,
		ExpiresInHours:  req.ExpiresInHours,
		JobSummaryShort: req.JobSummaryShort,
	}

	resp, err := a.service.CreateOfferFromQuote(ctx, tenantID, transportReq)
	if err != nil {
		return nil, err
	}

	return &ports.CreateOfferResult{
		OfferID:     resp.ID,
		PublicToken: resp.PublicToken,
		ExpiresAt:   resp.ExpiresAt.Format(time.RFC3339),
	}, nil
}
