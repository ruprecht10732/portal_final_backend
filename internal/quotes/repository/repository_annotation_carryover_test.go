package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMapQuoteItemsForAnnotationCarryoverPrefersExactMatches(t *testing.T) {
	productID := uuid.New()
	oldItem := QuoteItem{
		ID:               uuid.New(),
		Title:            "Warmtepomp",
		Description:      "Installatie woonkamer",
		Quantity:         "1",
		UnitPriceCents:   450000,
		TaxRateBps:       2100,
		IsOptional:       false,
		SortOrder:        0,
		CatalogProductID: &productID,
	}
	newItem := oldItem
	newItem.ID = uuid.New()

	mapping := mapQuoteItemsForAnnotationCarryover([]QuoteItem{oldItem}, []QuoteItem{newItem})
	if got := mapping[oldItem.ID]; got != newItem.ID {
		t.Fatalf("expected exact-match carryover to map %s to %s, got %s", oldItem.ID, newItem.ID, got)
	}
}

func TestMapQuoteItemsForAnnotationCarryoverFallsBackToCatalogIdentity(t *testing.T) {
	productID := uuid.New()
	oldItem := QuoteItem{
		ID:               uuid.New(),
		Title:            "Warmtepomp",
		Description:      "Oude omschrijving",
		Quantity:         "1",
		UnitPriceCents:   450000,
		TaxRateBps:       2100,
		IsOptional:       false,
		SortOrder:        0,
		CatalogProductID: &productID,
	}
	newItem := QuoteItem{
		ID:               uuid.New(),
		Title:            "Warmtepomp Plus",
		Description:      "Nieuwe omschrijving",
		Quantity:         "2",
		UnitPriceCents:   525000,
		TaxRateBps:       2100,
		IsOptional:       false,
		SortOrder:        1,
		CatalogProductID: &productID,
	}

	mapping := mapQuoteItemsForAnnotationCarryover([]QuoteItem{oldItem}, []QuoteItem{newItem})
	if got := mapping[oldItem.ID]; got != newItem.ID {
		t.Fatalf("expected catalog fallback to map %s to %s, got %s", oldItem.ID, newItem.ID, got)
	}
}

func TestMapQuoteItemsForAnnotationCarryoverFallsBackToSortOrder(t *testing.T) {
	oldItem := QuoteItem{
		ID:          uuid.New(),
		Description: "Dakinspectie",
		IsOptional:  true,
		SortOrder:   2,
	}
	newItem := QuoteItem{
		ID:          uuid.New(),
		Description: "Dakinspectie uitgebreid",
		IsOptional:  true,
		SortOrder:   2,
	}

	mapping := mapQuoteItemsForAnnotationCarryover([]QuoteItem{oldItem}, []QuoteItem{newItem})
	if got := mapping[oldItem.ID]; got != newItem.ID {
		t.Fatalf("expected sort-order fallback to map %s to %s, got %s", oldItem.ID, newItem.ID, got)
	}
}

func TestCloneQuoteAnnotationsForCarryoverDuplicatesMatchedAnnotations(t *testing.T) {
	fromItemID := uuid.New()
	toItemID := uuid.New()
	authorID := uuid.New()
	createdAt := time.Date(2026, time.March, 19, 10, 0, 0, 0, time.UTC)
	annotations := []QuoteAnnotation{{
		ID:             uuid.New(),
		QuoteItemID:    fromItemID,
		OrganizationID: uuid.New(),
		AuthorType:     "customer",
		AuthorID:       &authorID,
		Text:           "Kan dit stiller?",
		IsResolved:     false,
		CreatedAt:      createdAt,
	}}

	cloned := cloneQuoteAnnotationsForCarryover(annotations, map[uuid.UUID]uuid.UUID{fromItemID: toItemID})
	if len(cloned) != 1 {
		t.Fatalf("expected 1 cloned annotation, got %d", len(cloned))
	}
	if cloned[0].ID == annotations[0].ID {
		t.Fatal("expected cloned annotation to receive a new id")
	}
	if cloned[0].QuoteItemID != toItemID {
		t.Fatalf("expected cloned annotation to target %s, got %s", toItemID, cloned[0].QuoteItemID)
	}
	if cloned[0].Text != annotations[0].Text || cloned[0].AuthorType != annotations[0].AuthorType {
		t.Fatal("expected cloned annotation content to be preserved")
	}
	if !cloned[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("expected cloned annotation createdAt %v, got %v", createdAt, cloned[0].CreatedAt)
	}
}
