package service

import (
	"testing"

	"portal_final_backend/internal/catalog/repository"
)

func TestValidatePricingCreateAllowsZeroPricedDraft(t *testing.T) {
	svc := &Service{}

	unitLabel, err := svc.validatePricingCreate(0, 0, nil, true)
	if err != nil {
		t.Fatalf("expected draft pricing to allow empty placeholder values, got %v", err)
	}
	if unitLabel != nil {
		t.Fatal("expected nil unit label for zero-priced draft")
	}
}

func TestValidatePricingCreateRejectsZeroPricedPublishedProduct(t *testing.T) {
	svc := &Service{}

	if _, err := svc.validatePricingCreate(0, 0, nil, false); err == nil {
		t.Fatal("expected published product with no price to be rejected")
	}
}

func TestValidatePricingUpdateRejectsPublishedZeroPriceResult(t *testing.T) {
	svc := &Service{}
	current := repository.Product{PriceCents: 1250, UnitPriceCents: 0}
	price := int64(0)

	if _, err := svc.validatePricingUpdate(current, &price, nil, nil, false); err == nil {
		t.Fatal("expected published update with no effective price to be rejected")
	}
}

func TestValidatePricingUpdateAllowsDraftPlaceholder(t *testing.T) {
	svc := &Service{}
	current := repository.Product{IsDraft: true, PriceCents: 0, UnitPriceCents: 0}

	if _, err := svc.validatePricingUpdate(current, nil, nil, nil, true); err != nil {
		t.Fatalf("expected draft placeholder pricing to remain valid, got %v", err)
	}
}
