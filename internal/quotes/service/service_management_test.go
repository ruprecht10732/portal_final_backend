package service

import (
	"testing"
	"time"

	isdetransport "portal_final_backend/internal/isde/transport"
	"portal_final_backend/internal/quotes/transport"
)

func TestQuoteUpdateAffectsRenderedPDF(t *testing.T) {
	now := time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC)
	notes := "Updated note"
	pricingMode := "inclusive"
	discountType := "fixed"
	discountValue := int64(2500)
	financingDisclaimer := true
	pagePerItem := true
	items := []transport.QuoteItemRequest{{Description: "Warmtepomp", Quantity: "1", UnitPriceCents: 100000, TaxRateBps: 2100, IsSelected: true}}
	attachments := []transport.QuoteAttachmentRequest{{Filename: "datasheet.pdf", FileKey: "quotes/datasheet.pdf", Source: "manual", Enabled: true, SortOrder: 0}}
	urls := []transport.QuoteURLRequest{{Label: "Voorwaarden", Href: "https://example.com/voorwaarden"}}
	subsidy := &transport.QuoteISDESubsidy{IncludeInSummary: true, Result: &isdetransport.ISDECalculationResponse{TotalAmountCents: 125000}}

	tests := []struct {
		name string
		req  transport.UpdateQuoteRequest
		want bool
	}{
		{name: "no changes", req: transport.UpdateQuoteRequest{}, want: false},
		{name: "pricing mode", req: transport.UpdateQuoteRequest{PricingMode: &pricingMode}, want: true},
		{name: "discount type", req: transport.UpdateQuoteRequest{DiscountType: &discountType}, want: true},
		{name: "discount value", req: transport.UpdateQuoteRequest{DiscountValue: &discountValue}, want: true},
		{name: "valid until", req: transport.UpdateQuoteRequest{ValidUntil: &now}, want: true},
		{name: "notes", req: transport.UpdateQuoteRequest{Notes: &notes}, want: true},
		{name: "items", req: transport.UpdateQuoteRequest{Items: &items}, want: true},
		{name: "attachments", req: transport.UpdateQuoteRequest{Attachments: &attachments}, want: true},
		{name: "urls", req: transport.UpdateQuoteRequest{URLs: &urls}, want: true},
		{name: "subsidy", req: transport.UpdateQuoteRequest{ISDESubsidy: subsidy}, want: true},
		{name: "financing disclaimer", req: transport.UpdateQuoteRequest{FinancingDisclaimer: &financingDisclaimer}, want: true},
		{name: "page per item", req: transport.UpdateQuoteRequest{PagePerItem: &pagePerItem}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteUpdateAffectsRenderedPDF(tc.req)
			if got != tc.want {
				t.Fatalf("quoteUpdateAffectsRenderedPDF() = %v, want %v", got, tc.want)
			}
		})
	}
}
