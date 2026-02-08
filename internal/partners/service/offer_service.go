package service

import (
	"context"
	"encoding/json"
	"time"

	"portal_final_backend/internal/auth/token"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	offerTokenBytes = 32
	// platformFeeMultiplier: vakman receives 90% of customer price (10% platform fee).
	platformFeeMultiplier = 0.90
)

// CreateOffer generates a new job offer for a vakman based on customer pricing.
func (s *Service) CreateOffer(ctx context.Context, tenantID uuid.UUID, req transport.CreateOfferRequest) (transport.CreateOfferResponse, error) {
	// Validate partner belongs to tenant
	if err := s.ensurePartnerExists(ctx, tenantID, req.PartnerID); err != nil {
		return transport.CreateOfferResponse{}, err
	}

	// Validate lead service belongs to tenant
	if err := s.ensureLeadServiceExists(ctx, tenantID, req.LeadServiceID); err != nil {
		return transport.CreateOfferResponse{}, err
	}

	// Sequential model: only one active offer at a time
	hasActive, err := s.repo.HasActiveOffer(ctx, req.LeadServiceID)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}
	if hasActive {
		return transport.CreateOfferResponse{}, apperr.Conflict("an active offer already exists for this service")
	}

	// Calculate vakman earnings (90% of customer price)
	vakmanPrice := calculateVakmanPrice(req.CustomerPriceCents)

	// Generate secure random token
	rawToken, err := token.GenerateRandomToken(offerTokenBytes)
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	// Calculate expiry
	expiry := time.Now().UTC().Add(time.Duration(req.ExpiresInHours) * time.Hour)

	// Persist
	offer, err := s.repo.CreateOffer(ctx, repository.PartnerOffer{
		OrganizationID:     tenantID,
		PartnerID:          req.PartnerID,
		LeadServiceID:      req.LeadServiceID,
		PublicToken:        rawToken,
		ExpiresAt:          expiry,
		PricingSource:      req.PricingSource,
		CustomerPriceCents: req.CustomerPriceCents,
		VakmanPriceCents:   vakmanPrice,
	})
	if err != nil {
		return transport.CreateOfferResponse{}, err
	}

	// Publish event for timeline & notification handlers
	s.eventBus.Publish(ctx, events.PartnerOfferCreated{
		BaseEvent:        events.NewBaseEvent(),
		OfferID:          offer.ID,
		OrganizationID:   tenantID,
		PartnerID:        req.PartnerID,
		LeadServiceID:    req.LeadServiceID,
		VakmanPriceCents: vakmanPrice,
		PublicToken:      rawToken,
	})

	return transport.CreateOfferResponse{
		ID:               offer.ID,
		PublicToken:      rawToken,
		VakmanPriceCents: vakmanPrice,
		ExpiresAt:        expiry,
	}, nil
}

// GetPublicOffer retrieves offer details for the vakman-facing view.
// Only exposes the vakman's price — customer markup is never served.
func (s *Service) GetPublicOffer(ctx context.Context, publicToken string) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	return transport.PublicOfferResponse{
		OfferID:          oc.ID,
		OrganizationName: oc.OrganizationName,
		JobSummary:       oc.ServiceType,
		City:             oc.LeadCity,
		VakmanPriceCents: oc.VakmanPriceCents,
		PricingSource:    oc.PricingSource,
		Status:           oc.Status,
		ExpiresAt:        oc.ExpiresAt,
		CreatedAt:        oc.CreatedAt,
	}, nil
}

// AcceptOffer processes a vakman's acceptance, locks the job via the unique index.
func (s *Service) AcceptOffer(ctx context.Context, publicToken string, req transport.AcceptOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	// Validation
	if time.Now().After(oc.ExpiresAt) {
		return apperr.Gone("this offer has expired")
	}
	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be accepted in current state")
	}

	// Serialize availability slots
	inspectionJSON, err := json.Marshal(req.InspectionSlots)
	if err != nil {
		return apperr.Validation("invalid inspection slots")
	}

	var jobJSON []byte
	if len(req.JobSlots) > 0 {
		jobJSON, err = json.Marshal(req.JobSlots)
		if err != nil {
			return apperr.Validation("invalid job slots")
		}
	}

	// Atomic update (unique index enforces exclusivity)
	if err := s.repo.AcceptOffer(ctx, oc.ID, inspectionJSON, jobJSON); err != nil {
		return err
	}

	// Publish event
	s.eventBus.Publish(ctx, events.PartnerOfferAccepted{
		BaseEvent:      events.NewBaseEvent(),
		OfferID:        oc.ID,
		OrganizationID: oc.OrganizationID,
		PartnerID:      oc.PartnerID,
		LeadServiceID:  oc.LeadServiceID,
	})

	return nil
}

// RejectOffer processes a vakman's rejection of an offer.
func (s *Service) RejectOffer(ctx context.Context, publicToken string, req transport.RejectOfferRequest) error {
	oc, err := s.repo.GetOfferByToken(ctx, publicToken)
	if err != nil {
		return err
	}

	if oc.Status != "pending" && oc.Status != "sent" {
		return apperr.Conflict("offer cannot be rejected in current state")
	}

	if err := s.repo.RejectOffer(ctx, oc.ID, req.Reason); err != nil {
		return err
	}

	s.eventBus.Publish(ctx, events.PartnerOfferRejected{
		BaseEvent:      events.NewBaseEvent(),
		OfferID:        oc.ID,
		OrganizationID: oc.OrganizationID,
		PartnerID:      oc.PartnerID,
		LeadServiceID:  oc.LeadServiceID,
		Reason:         req.Reason,
	})

	return nil
}

// GetOfferPreview returns the same vakman-facing view but requires authentication.
// This lets admin users preview what the vakman sees.
func (s *Service) GetOfferPreview(ctx context.Context, tenantID uuid.UUID, offerID uuid.UUID) (transport.PublicOfferResponse, error) {
	oc, err := s.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return transport.PublicOfferResponse{}, err
	}

	return transport.PublicOfferResponse{
		OfferID:          oc.ID,
		OrganizationName: oc.OrganizationName,
		JobSummary:       oc.ServiceType,
		City:             oc.LeadCity,
		VakmanPriceCents: oc.VakmanPriceCents,
		PricingSource:    oc.PricingSource,
		Status:           oc.Status,
		ExpiresAt:        oc.ExpiresAt,
		CreatedAt:        oc.CreatedAt,
	}, nil
}

// ListOffersForService returns all offers for a given lead service (admin view).
func (s *Service) ListOffersForService(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID) (transport.ListOffersResponse, error) {
	offers, err := s.repo.ListOffersForService(ctx, leadServiceID, tenantID)
	if err != nil {
		return transport.ListOffersResponse{}, err
	}

	items := make([]transport.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, mapOfferResponse(o))
	}

	return transport.ListOffersResponse{Items: items}, nil
}

// ExpireOffers is called by a background job to expire stale offers.
func (s *Service) ExpireOffers(ctx context.Context) (int, error) {
	expired, err := s.repo.ExpireOffers(ctx)
	if err != nil {
		return 0, err
	}

	for _, o := range expired {
		s.eventBus.Publish(ctx, events.PartnerOfferExpired{
			BaseEvent:      events.NewBaseEvent(),
			OfferID:        o.ID,
			OrganizationID: o.OrganizationID,
			PartnerID:      o.PartnerID,
			LeadServiceID:  o.LeadServiceID,
		})
	}

	return len(expired), nil
}

func mapOfferResponse(oc repository.PartnerOfferWithContext) transport.OfferResponse {
	resp := transport.OfferResponse{
		ID:                 oc.ID,
		PartnerID:          oc.PartnerID,
		PartnerName:        oc.PartnerName,
		LeadServiceID:      oc.LeadServiceID,
		PricingSource:      oc.PricingSource,
		CustomerPriceCents: oc.CustomerPriceCents,
		VakmanPriceCents:   oc.VakmanPriceCents,
		Status:             oc.Status,
		PublicToken:        oc.PublicToken,
		ExpiresAt:          oc.ExpiresAt,
		AcceptedAt:         oc.AcceptedAt,
		RejectedAt:         oc.RejectedAt,
		CreatedAt:          oc.CreatedAt,
	}
	if oc.RejectionReason != nil {
		resp.RejectionReason = *oc.RejectionReason
	}
	return resp
}

func calculateVakmanPrice(customerPriceCents int64) int64 {
	return int64(float64(customerPriceCents) * platformFeeMultiplier)
}
