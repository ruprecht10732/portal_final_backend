package service

import (
	"testing"

	"portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

func TestBuildQuoteVersionDiffMarksChangedAddedAndRemovedItems(t *testing.T) {
	productID := uuid.New()
	previousQuote := &repository.Quote{ID: uuid.New(), QuoteNumber: "OFF-2026-0001", VersionNumber: 1, PricingMode: "exclusive", TotalCents: 100000}
	currentQuote := &repository.Quote{ID: uuid.New(), QuoteNumber: "OFF-2026-0002", VersionNumber: 2, PricingMode: "exclusive", TotalCents: 140000}
	previousItems := []repository.QuoteItem{
		{ID: uuid.New(), Title: "Warmtepomp", Quantity: "1", UnitPriceCents: 100000, TaxRateBps: 2100, SortOrder: 0, CatalogProductID: &productID},
		{ID: uuid.New(), Title: "Oude optie", Quantity: "1", UnitPriceCents: 5000, TaxRateBps: 2100, SortOrder: 1, IsOptional: true},
	}
	currentItems := []repository.QuoteItem{
		{ID: uuid.New(), Title: "Warmtepomp", Quantity: "1", UnitPriceCents: 120000, TaxRateBps: 2100, SortOrder: 0, CatalogProductID: &productID},
		{ID: uuid.New(), Title: "Nieuwe optie", Quantity: "1", UnitPriceCents: 15000, TaxRateBps: 2100, SortOrder: 2, IsOptional: true},
	}

	diff := buildQuoteVersionDiff(previousQuote, previousItems, currentQuote, currentItems)
	if diff != nil {
		if diff.ChangedCount != 1 || diff.RemovedCount != 1 || diff.AddedCount != 1 {
			t.Fatalf("unexpected change counts: %+v", diff)
		}
		if diff.TotalDeltaCents != 40000 {
			t.Fatalf("expected total delta 40000, got %d", diff.TotalDeltaCents)
		}
		if len(diff.Items) != 3 {
			t.Fatalf("expected 3 diff items, got %d", len(diff.Items))
		}
		return
	}
	t.Fatal("expected diff response")
}
