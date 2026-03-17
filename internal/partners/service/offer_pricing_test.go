package service

import (
	"context"
	"testing"

	"portal_final_backend/internal/partners/repository"

	"github.com/google/uuid"
)

func TestResolveOfferMarginBasisPointsUsesOverride(t *testing.T) {
	svc := &Service{}
	override := 1750

	margin := svc.resolveOfferMarginBasisPoints(context.Background(), uuid.New(), &override)

	if margin != 1750 {
		t.Fatalf("expected override margin 1750, got %d", margin)
	}
}

func TestResolveOfferMarginBasisPointsUsesOrganizationSetting(t *testing.T) {
	tenantID := uuid.New()
	svc := &Service{
		settingsReader: func(_ context.Context, organizationID uuid.UUID) (OrganizationOfferSettings, error) {
			if organizationID != tenantID {
				t.Fatalf("expected organization id %s, got %s", tenantID, organizationID)
			}
			return OrganizationOfferSettings{OfferMarginBasisPoints: 2250}, nil
		},
	}

	margin := svc.resolveOfferMarginBasisPoints(context.Background(), tenantID, nil)

	if margin != 2250 {
		t.Fatalf("expected organization margin 2250, got %d", margin)
	}
}

func TestResolveOfferMarginBasisPointsFallsBackToDefault(t *testing.T) {
	svc := &Service{}

	margin := svc.resolveOfferMarginBasisPoints(context.Background(), uuid.New(), nil)

	if margin != defaultOfferMarginBasisPoints {
		t.Fatalf("expected default margin %d, got %d", defaultOfferMarginBasisPoints, margin)
	}
}

func TestResolveVakmanPriceUsesMarginOrOverride(t *testing.T) {
	calculated := resolveVakmanPrice(20000, 1250, nil)
	if calculated != 17500 {
		t.Fatalf("expected calculated vakman price 17500, got %d", calculated)
	}

	override := int64(14321)
	manual := resolveVakmanPrice(20000, 1250, &override)
	if manual != 14321 {
		t.Fatalf("expected override vakman price 14321, got %d", manual)
	}
}

func TestSelectOfferItemsFiltersBySelectedIDs(t *testing.T) {
	keepID := uuid.New()
	skipID := uuid.New()
	items := []repository.QuoteItemSummary{
		{ID: keepID, Description: "Keep", LineTotalCents: 1000},
		{ID: skipID, Description: "Skip", LineTotalCents: 2500},
	}

	selected := selectOfferItems(items, []uuid.UUID{keepID})

	if len(selected) != 1 {
		t.Fatalf("expected 1 selected item, got %d", len(selected))
	}
	if selected[0].ID != keepID {
		t.Fatalf("expected kept item %s, got %s", keepID, selected[0].ID)
	}
}

func TestBuildOfferLineItemsAndCustomerPriceUseSelectedItems(t *testing.T) {
	itemID := uuid.New()
	items := []repository.QuoteItemSummary{{
		ID:             itemID,
		Description:    "Roof repair",
		Quantity:       "2x",
		UnitPriceCents: 4500,
		LineTotalCents: 9000,
	}}

	lineItems := buildOfferLineItems(items)
	if len(lineItems) != 1 {
		t.Fatalf("expected 1 line item, got %d", len(lineItems))
	}
	if lineItems[0].QuoteItemID != itemID {
		t.Fatalf("expected quote item id %s, got %s", itemID, lineItems[0].QuoteItemID)
	}

	total := calculateCustomerPrice(items)
	if total != 9000 {
		t.Fatalf("expected total 9000, got %d", total)
	}
}