package service

import (
	"context"

	"github.com/google/uuid"

	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/internal/scheduler"
)

func (s *Service) SubmitHumanFeedback(ctx context.Context, quoteID uuid.UUID, tenantID uuid.UUID, req transport.CreateHumanFeedbackRequest) (*transport.HumanFeedbackResponse, error) {
	quote, err := s.repo.GetByID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, err
	}

	leadServiceID, err := resolveFeedbackLeadServiceID(quote.LeadServiceID, req.LeadServiceID, func(id uuid.UUID) error {
		return s.repo.ValidateLeadServiceID(ctx, quoteID, tenantID, id)
	})
	if err != nil {
		return nil, err
	}

	feedback, err := s.repo.CreateHumanFeedback(ctx, repository.CreateHumanFeedbackParams{
		OrganizationID: tenantID,
		QuoteID:        quoteID,
		LeadServiceID:  leadServiceID,
		FieldChanged:   req.FieldChanged,
		AIValue:        req.AIValue,
		HumanValue:     req.HumanValue,
	})
	if err != nil {
		return nil, err
	}

	// Persist the correction even if async memory enrichment cannot be queued.
	// Returning an error here would encourage retries and duplicate feedback rows.
	if s.feedbackQueue != nil {
		_ = s.feedbackQueue.EnqueueApplyHumanFeedbackMemory(ctx, scheduler.ApplyHumanFeedbackMemoryPayload{
			TenantID:   tenantID.String(),
			FeedbackID: feedback.ID.String(),
		})
	}

	return &transport.HumanFeedbackResponse{
		ID:              feedback.ID,
		QuoteID:         feedback.QuoteID,
		LeadServiceID:   feedback.LeadServiceID,
		FieldChanged:    feedback.FieldChanged,
		DeltaPercentage: feedback.DeltaPercentage,
		AppliedToMemory: feedback.AppliedToMemory,
		CreatedAt:       feedback.CreatedAt,
	}, nil
}

func resolveFeedbackLeadServiceID(
	quoteLeadServiceID *uuid.UUID,
	requestedLeadServiceID *uuid.UUID,
	validate func(uuid.UUID) error,
) (*uuid.UUID, error) {
	if requestedLeadServiceID == nil {
		return quoteLeadServiceID, nil
	}
	if err := validate(*requestedLeadServiceID); err != nil {
		return nil, err
	}
	return requestedLeadServiceID, nil
}
