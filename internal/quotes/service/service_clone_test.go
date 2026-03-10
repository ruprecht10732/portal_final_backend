package service

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/platform/apperr"
)

func TestValidateVersionSourceStatusAcceptsAcceptedAndRejected(t *testing.T) {
	for _, status := range []string{"Accepted", "Rejected"} {
		if err := validateVersionSourceStatus(status); err != nil {
			t.Fatalf("validateVersionSourceStatus(%q) returned error: %v", status, err)
		}
	}
}

func TestValidateVersionSourceStatusRejectsNonTerminalStatuses(t *testing.T) {
	for _, status := range []string{"Draft", "Sent", "Expired"} {
		if err := validateVersionSourceStatus(status); err == nil {
			t.Fatalf("expected error for status %q", status)
		} else if !apperr.Is(err, apperr.KindBadRequest) {
			t.Fatalf("expected bad request for status %q, got %v", status, err)
		}
	}
}

func TestCloneQuoteValidUntilDropsExpiredDate(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	if result := cloneQuoteValidUntil(now, &expired); result != nil {
		t.Fatalf("expected expired validUntil to be cleared")
	}
}

func TestCloneQuoteValidUntilPreservesFutureDate(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	result := cloneQuoteValidUntil(now, &future)
	if result == nil {
		t.Fatal("expected future validUntil to be preserved")
	}
	if !result.Equal(future) {
		t.Fatalf("expected %v, got %v", future, *result)
	}
	if result == &future {
		t.Fatal("expected cloneQuoteValidUntil to return a copied time pointer")
	}
}

func TestCloneQuoteItemsCopiesFieldsAndAssignsNewIDs(t *testing.T) {
	quoteID := uuid.New()
	orgID := uuid.New()
	productID := uuid.New()
	createdAt := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	source := []repository.QuoteItem{{
		ID:               uuid.New(),
		QuoteID:          uuid.New(),
		OrganizationID:   orgID,
		Title:            "Solar panels",
		Description:      "Install 10 panels",
		Quantity:         "10 x",
		QuantityNumeric:  10,
		UnitPriceCents:   125000,
		TaxRateBps:       2100,
		IsOptional:       true,
		IsSelected:       true,
		SortOrder:        3,
		CatalogProductID: &productID,
		CreatedAt:        createdAt.Add(-time.Hour),
	}}

	cloned := cloneQuoteItems(quoteID, orgID, createdAt, source)
	if len(cloned) != 1 {
		t.Fatalf("expected 1 cloned item, got %d", len(cloned))
	}
	if cloned[0].ID == source[0].ID {
		t.Fatal("expected cloned item to get a new ID")
	}
	if cloned[0].QuoteID != quoteID {
		t.Fatalf("expected quote id %s, got %s", quoteID, cloned[0].QuoteID)
	}
	if cloned[0].OrganizationID != orgID {
		t.Fatalf("expected org id %s, got %s", orgID, cloned[0].OrganizationID)
	}
	if cloned[0].Title != source[0].Title || cloned[0].Description != source[0].Description || cloned[0].Quantity != source[0].Quantity {
		t.Fatal("expected cloned item fields to match source")
	}
	if cloned[0].CreatedAt != createdAt {
		t.Fatalf("expected createdAt %v, got %v", createdAt, cloned[0].CreatedAt)
	}
	if cloned[0].CatalogProductID == nil || *cloned[0].CatalogProductID != productID {
		t.Fatal("expected catalog product id to be preserved")
	}
}
