package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/platform/apperr"
)

func TestResolveFeedbackLeadServiceIDUsesQuoteServiceWhenRequestOmitted(t *testing.T) {
	quoteLeadServiceID := uuid.New()
	called := false

	resolved, err := resolveFeedbackLeadServiceID(&quoteLeadServiceID, nil, func(id uuid.UUID) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called {
		t.Fatalf("expected validator not to be called when request lead service id is omitted")
	}
	if resolved == nil || *resolved != quoteLeadServiceID {
		t.Fatalf("expected resolved lead service id to equal quote lead service id")
	}
}

func TestResolveFeedbackLeadServiceIDValidatesRequestedService(t *testing.T) {
	requestedLeadServiceID := uuid.New()
	called := false

	resolved, err := resolveFeedbackLeadServiceID(nil, &requestedLeadServiceID, func(id uuid.UUID) error {
		called = true
		if id != requestedLeadServiceID {
			t.Fatalf("validator received unexpected id %s", id)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected validator to be called")
	}
	if resolved == nil || *resolved != requestedLeadServiceID {
		t.Fatalf("expected requested lead service id to be returned")
	}
}

func TestResolveFeedbackLeadServiceIDReturnsValidationError(t *testing.T) {
	requestedLeadServiceID := uuid.New()
	validationErr := apperr.Validation("leadServiceId does not belong to this quote")

	resolved, err := resolveFeedbackLeadServiceID(nil, &requestedLeadServiceID, func(id uuid.UUID) error {
		return validationErr
	})
	if resolved != nil {
		t.Fatalf("expected no resolved lead service id on validation failure")
	}
	if !errors.Is(err, validationErr) {
		t.Fatalf("expected validation error, got %v", err)
	}
}