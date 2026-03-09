package agent

import "testing"

func TestNormalizeDraftQuoteInputDefaultsMissingQuantities(t *testing.T) {
	input := DraftQuoteInput{
		Notes: "  conceptofferte  ",
		Items: []DraftQuoteItem{
			{Description: "  Poortdeur vervangen  ", Quantity: "", UnitPriceCents: 1000, TaxRateBps: 2100},
			{Description: "Montage", Quantity: " 2 uur ", UnitPriceCents: 2000, TaxRateBps: 2100},
		},
	}

	normalized, corrections := normalizeDraftQuoteInput(input)

	if normalized.Notes != "conceptofferte" {
		t.Fatalf("expected notes to be trimmed, got %q", normalized.Notes)
	}
	if normalized.Items[0].Quantity != "1" {
		t.Fatalf("expected first quantity to default to 1, got %q", normalized.Items[0].Quantity)
	}
	if normalized.Items[0].Description != "Poortdeur vervangen" {
		t.Fatalf("expected first description to be trimmed, got %q", normalized.Items[0].Description)
	}
	if normalized.Items[1].Quantity != "2 uur" {
		t.Fatalf("expected second quantity to be trimmed, got %q", normalized.Items[1].Quantity)
	}
	if len(corrections) != 1 {
		t.Fatalf("expected 1 quantity correction, got %d", len(corrections))
	}
	if corrections[0].Index != 0 {
		t.Fatalf("expected correction for first item, got index %d", corrections[0].Index)
	}
	if corrections[0].Description != "Poortdeur vervangen" {
		t.Fatalf("expected correction description to be trimmed, got %q", corrections[0].Description)
	}
}

func TestNormalizeDraftQuoteItemLeavesExplicitQuantityUntouched(t *testing.T) {
	item, corrected := normalizeDraftQuoteItem(DraftQuoteItem{
		Description: "  Glas vervangen ",
		Quantity:    " 3 stuks ",
	})

	if corrected {
		t.Fatal("expected explicit quantity to remain unchanged")
	}
	if item.Quantity != "3 stuks" {
		t.Fatalf("expected trimmed quantity, got %q", item.Quantity)
	}
	if item.Description != "Glas vervangen" {
		t.Fatalf("expected trimmed description, got %q", item.Description)
	}
}

func TestFindInvalidDraftQuoteQuantityRejectsPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		quantity string
	}{
		{name: "question mark", quantity: "?"},
		{name: "uncertain suffix", quantity: "2 stuks?"},
		{name: "nader te bepalen", quantity: " nader te bepalen "},
		{name: "ntb punctuation", quantity: "N.T.B."},
		{name: "tbd", quantity: "tbd"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid, ok := findInvalidDraftQuoteQuantity([]DraftQuoteItem{{Description: "Regel", Quantity: test.quantity}})
			if !ok {
				t.Fatalf("expected quantity %q to be rejected", test.quantity)
			}
			if invalid.Index != 0 {
				t.Fatalf("expected invalid quantity index 0, got %d", invalid.Index)
			}
			if invalid.Quantity != test.quantity {
				t.Fatalf("expected invalid quantity %q, got %q", test.quantity, invalid.Quantity)
			}
		})
	}
}

func TestFindInvalidDraftQuoteQuantityAllowsConcreteValues(t *testing.T) {
	items := []DraftQuoteItem{
		{Description: "Poort", Quantity: "1"},
		{Description: "Montage", Quantity: "3 uur"},
		{Description: "Latten", Quantity: "6 meter"},
	}

	if invalid, ok := findInvalidDraftQuoteQuantity(items); ok {
		t.Fatalf("expected concrete quantities to pass, got invalid %+v", invalid)
	}
}
