package repository

import (
	"strings"
	"testing"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	buildPricingCorrectionsErrFmt = "buildPricingCorrections returned error: %v"
	testQuoteItemSolarPanel       = "Solar panel"
	testQuoteItemBattery          = "Battery"
	testQuoteItemCustomWork       = "Custom work"
	testPriceBandMidRange         = "1000_2500"
)

func TestBuildPricingCorrectionsIgnoresPureReorder(t *testing.T) {
	productA := uuid.New()
	productB := uuid.New()
	quote := &Quote{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DiscountValue:  0,
		SubtotalCents:  3000,
		TaxTotalCents:  0,
		TotalCents:     3000,
	}
	previousItems := []QuotePricingSnapshotItem{
		{Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000, CatalogProductID: &productA},
		{Description: testQuoteItemBattery, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000, CatalogProductID: &productB},
	}
	structuredItems := marshalJSON(previousItems)
	previousSnapshot := quotesdb.RacQuotePricingSnapshot{
		ItemCount:       2,
		DiscountValue:   0,
		SubtotalCents:   3000,
		TaxTotalCents:   0,
		TotalCents:      3000,
		StructuredItems: structuredItems,
	}
	currentItems := []QuoteItem{
		{ID: uuid.New(), Description: testQuoteItemBattery, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000, CatalogProductID: &productB},
		{ID: uuid.New(), Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000, CatalogProductID: &productA},
	}

	corrections, err := buildPricingCorrections(previousSnapshot, quote, currentItems)
	if err != nil {
		t.Fatalf(buildPricingCorrectionsErrFmt, err)
	}
	if len(corrections) != 0 {
		t.Fatalf("expected no corrections for pure reorder, got %d", len(corrections))
	}
}

func TestBuildPricingCorrectionsKeepsMatchedItemAdjustment(t *testing.T) {
	productID := uuid.New()
	quote := &Quote{
		ID:                  uuid.New(),
		OrganizationID:      uuid.New(),
		DiscountValue:       0,
		SubtotalCents:       1500,
		DiscountAmountCents: 0,
		TaxTotalCents:       0,
		TotalCents:          1500,
	}
	previousSnapshot := quotesdb.RacQuotePricingSnapshot{
		ItemCount:       1,
		DiscountValue:   0,
		SubtotalCents:   1000,
		TaxTotalCents:   0,
		TotalCents:      1000,
		StructuredItems: marshalJSON([]QuotePricingSnapshotItem{{Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000, TaxRateBps: 0, CatalogProductID: &productID}}),
	}
	currentItems := []QuoteItem{{ID: uuid.New(), Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1500, TaxRateBps: 0, CatalogProductID: &productID}}

	corrections, err := buildPricingCorrections(previousSnapshot, quote, currentItems)
	if err != nil {
		t.Fatalf(buildPricingCorrectionsErrFmt, err)
	}

	hasUnitPriceCorrection := false
	for _, correction := range corrections {
		if correction.FieldName == "items[0].unit_price_cents" {
			hasUnitPriceCorrection = true
			break
		}
	}
	if !hasUnitPriceCorrection {
		t.Fatalf("expected a unit price correction, got %#v", corrections)
	}
}

func TestBuildPricingCorrectionsDetectsAddedItem(t *testing.T) {
	productID := uuid.New()
	quote := &Quote{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DiscountValue:  0,
		SubtotalCents:  3000,
		TaxTotalCents:  0,
		TotalCents:     3000,
	}
	previousSnapshot := quotesdb.RacQuotePricingSnapshot{
		ItemCount:       1,
		DiscountValue:   0,
		SubtotalCents:   1000,
		TaxTotalCents:   0,
		TotalCents:      1000,
		StructuredItems: marshalJSON([]QuotePricingSnapshotItem{{Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000, CatalogProductID: &productID}}),
	}
	currentItems := []QuoteItem{
		{ID: uuid.New(), Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000, CatalogProductID: &productID},
		{ID: uuid.New(), Description: testQuoteItemBattery, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000},
	}

	corrections, err := buildPricingCorrections(previousSnapshot, quote, currentItems)
	if err != nil {
		t.Fatalf(buildPricingCorrectionsErrFmt, err)
	}

	assertHasCorrectionCode(t, corrections, "item_added")
	assertHasCorrectionCode(t, corrections, "item_count_changed")
}

func TestBuildPricingCorrectionsDetectsRemovedItem(t *testing.T) {
	quote := &Quote{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DiscountValue:  0,
		SubtotalCents:  1000,
		TaxTotalCents:  0,
		TotalCents:     1000,
	}
	previousSnapshot := quotesdb.RacQuotePricingSnapshot{
		ItemCount:     2,
		DiscountValue: 0,
		SubtotalCents: 3000,
		TaxTotalCents: 0,
		TotalCents:    3000,
		StructuredItems: marshalJSON([]QuotePricingSnapshotItem{
			{Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000},
			{Description: testQuoteItemBattery, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000},
		}),
	}
	currentItems := []QuoteItem{{ID: uuid.New(), Description: testQuoteItemSolarPanel, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000}}

	corrections, err := buildPricingCorrections(previousSnapshot, quote, currentItems)
	if err != nil {
		t.Fatalf(buildPricingCorrectionsErrFmt, err)
	}

	assertHasCorrectionCode(t, corrections, "item_removed")
	assertHasCorrectionCode(t, corrections, "item_count_changed")
}

func TestBuildPricingCorrectionsHandlesDuplicateDescriptionAdHocItemsConservatively(t *testing.T) {
	quote := &Quote{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DiscountValue:  0,
		SubtotalCents:  4500,
		TaxTotalCents:  0,
		TotalCents:     4500,
	}
	previousSnapshot := quotesdb.RacQuotePricingSnapshot{
		ItemCount:     2,
		DiscountValue: 0,
		SubtotalCents: 3000,
		TaxTotalCents: 0,
		TotalCents:    3000,
		StructuredItems: marshalJSON([]QuotePricingSnapshotItem{
			{Description: testQuoteItemCustomWork, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 1000},
			{Description: testQuoteItemCustomWork, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000},
		}),
	}
	currentItems := []QuoteItem{
		{ID: uuid.New(), Description: testQuoteItemCustomWork, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2000},
		{ID: uuid.New(), Description: testQuoteItemCustomWork, Quantity: "1", QuantityNumeric: 1, UnitPriceCents: 2500},
	}

	corrections, err := buildPricingCorrections(previousSnapshot, quote, currentItems)
	if err != nil {
		t.Fatalf(buildPricingCorrectionsErrFmt, err)
	}

	assertHasCorrectionCode(t, corrections, "item_added")
	assertHasCorrectionCode(t, corrections, "item_removed")
	assertNoFieldSuffix(t, corrections, ".unit_price_cents")
}

func TestListPricingIntelligenceAggregatesMapsOutcomePointerFromCount(t *testing.T) {
	withOutcome := quotesdb.ListPricingIntelligenceAggregatesRow{
		RegionPrefix:        "1234",
		PriceBand:           testPriceBandMidRange,
		SampleCount:         2,
		AcceptedCount:       1,
		RejectedCount:       1,
		ConversionRate:      50,
		AverageQuotedCents:  150000,
		AverageOutcomeCents: 125000,
		OutcomeTotalCount:   1,
	}
	withoutOutcome := quotesdb.ListPricingIntelligenceAggregatesRow{
		RegionPrefix:        "1234",
		PriceBand:           testPriceBandMidRange,
		SampleCount:         1,
		AcceptedCount:       1,
		RejectedCount:       0,
		ConversionRate:      100,
		AverageQuotedCents:  150000,
		AverageOutcomeCents: 0,
		OutcomeTotalCount:   0,
	}

	got := mapPricingIntelligenceAggregates([]quotesdb.ListPricingIntelligenceAggregatesRow{withOutcome, withoutOutcome})
	if len(got) != 2 {
		t.Fatalf("expected 2 aggregates, got %d", len(got))
	}
	if got[0].AverageOutcomeCents == nil || *got[0].AverageOutcomeCents != 125000 {
		t.Fatalf("expected first aggregate outcome average pointer to be set, got %#v", got[0].AverageOutcomeCents)
	}
	if got[1].AverageOutcomeCents != nil {
		t.Fatalf("expected second aggregate outcome average pointer to be nil, got %#v", got[1].AverageOutcomeCents)
	}
}

func TestOptionalInt64ReturnsNilForInvalid(t *testing.T) {
	if value := optionalInt64(pgtype.Int8{}); value != nil {
		t.Fatalf("expected nil for invalid pgtype.Int8, got %#v", value)
	}
}

func assertHasCorrectionCode(t *testing.T, corrections []quotePricingCorrection, code string) {
	t.Helper()
	for _, correction := range corrections {
		if correction.AIFindingCode != nil && *correction.AIFindingCode == code {
			return
		}
	}
	t.Fatalf("expected correction with code %q, got %#v", code, corrections)
}

func assertNoFieldSuffix(t *testing.T, corrections []quotePricingCorrection, suffix string) {
	t.Helper()
	for _, correction := range corrections {
		if strings.HasSuffix(correction.FieldName, suffix) {
			t.Fatalf("did not expect correction field suffix %q, got %#v", suffix, corrections)
		}
	}
}
