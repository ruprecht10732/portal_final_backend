package service

import (
	"testing"

	"portal_final_backend/internal/quotes/transport"
)

func TestCalculateQuote_DiscountDoesNotReduceVAT_ExclusivePricing(t *testing.T) {
	req := transport.QuoteCalculationRequest{
		PricingMode:   "exclusive",
		DiscountType:  "fixed",
		DiscountValue: 1000,
		Items: []transport.QuoteItemRequest{
			{
				Description:    "rest",
				Quantity:       "1",
				UnitPriceCents: 10000,
				TaxRateBps:     2100,
			},
		},
	}

	result := CalculateQuote(req)

	if result.SubtotalCents != 10000 {
		t.Fatalf("expected subtotal 10000, got %d", result.SubtotalCents)
	}
	if result.DiscountAmountCents != 1000 {
		t.Fatalf("expected discount 1000, got %d", result.DiscountAmountCents)
	}
	if result.VatTotalCents != 2100 {
		t.Fatalf("expected VAT 2100, got %d", result.VatTotalCents)
	}
	if result.TotalCents != 11100 {
		t.Fatalf("expected total 11100, got %d", result.TotalCents)
	}
	if len(result.VatBreakdown) != 1 {
		t.Fatalf("expected 1 VAT breakdown line, got %d", len(result.VatBreakdown))
	}
	if result.VatBreakdown[0].RateBps != 2100 || result.VatBreakdown[0].AmountCents != 2100 {
		t.Fatalf("expected VAT breakdown 2100=>2100, got %d=>%d", result.VatBreakdown[0].RateBps, result.VatBreakdown[0].AmountCents)
	}
}

func TestCalculateQuote_DiscountDoesNotReduceVAT_InclusivePricing(t *testing.T) {
	req := transport.QuoteCalculationRequest{
		PricingMode:   "inclusive",
		DiscountType:  "fixed",
		DiscountValue: 1000,
		Items: []transport.QuoteItemRequest{
			{
				Description:    "rest",
				Quantity:       "1",
				UnitPriceCents: 12100,
				TaxRateBps:     2100,
			},
		},
	}

	result := CalculateQuote(req)

	if result.SubtotalCents != 10000 {
		t.Fatalf("expected subtotal 10000, got %d", result.SubtotalCents)
	}
	if result.DiscountAmountCents != 1000 {
		t.Fatalf("expected discount 1000, got %d", result.DiscountAmountCents)
	}
	if result.VatTotalCents != 2100 {
		t.Fatalf("expected VAT 2100, got %d", result.VatTotalCents)
	}
	if result.TotalCents != 11100 {
		t.Fatalf("expected total 11100, got %d", result.TotalCents)
	}
}
