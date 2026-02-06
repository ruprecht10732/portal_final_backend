package service

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"portal_final_backend/internal/quotes/transport"
)

var quantityRegex = regexp.MustCompile(`^([\d.,]+)`)

// parseQuantityNumber extracts numeric value from free-form quantity string.
// Examples: "5 x" -> 5.0, "10 m²" -> 10.0, "3.5 uur" -> 3.5
func parseQuantityNumber(quantity string) float64 {
	matches := quantityRegex.FindStringSubmatch(strings.TrimSpace(quantity))
	if len(matches) < 2 {
		return 1.0
	}
	// Support Dutch comma decimal separator
	cleaned := strings.ReplaceAll(matches[1], ",", ".")
	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || val <= 0 {
		return 1.0
	}
	return val
}

// roundCents rounds a float to the nearest cent (integer)
func roundCents(v float64) int64 {
	return int64(math.Round(v))
}

// computeLineNetPrice returns the net (excl. tax) unit price given the pricing mode.
func computeLineNetPrice(unitPriceCents int64, taxRateBps int, pricingMode string) float64 {
	price := float64(unitPriceCents)
	if pricingMode == "inclusive" && taxRateBps > 0 {
		price /= 1.0 + float64(taxRateBps)/10000.0
	}
	return price
}

// computeDiscount returns the discount amount in float-cents, capped at the subtotal.
func computeDiscount(subtotalFloat float64, discountType string, discountValue int64) float64 {
	var amount float64
	switch {
	case discountType == "percentage" && discountValue > 0:
		amount = subtotalFloat * (float64(discountValue) / 100.0)
	case discountType == "fixed" && discountValue > 0:
		amount = float64(discountValue)
	}
	if amount > subtotalFloat {
		return subtotalFloat
	}
	return amount
}

// computeVatBreakdown applies the proportional discount multiplier to each VAT rate
// and returns the total VAT in cents plus a sorted breakdown slice.
func computeVatBreakdown(vatMap map[int]float64, multiplier float64) (int64, []transport.VatBreakdown) {
	var vatTotal int64
	breakdown := make([]transport.VatBreakdown, 0, len(vatMap))
	for rate, amount := range vatMap {
		adjusted := roundCents(amount * multiplier)
		vatTotal += adjusted
		breakdown = append(breakdown, transport.VatBreakdown{RateBps: rate, AmountCents: adjusted})
	}
	sort.Slice(breakdown, func(i, j int) bool { return breakdown[i].RateBps < breakdown[j].RateBps })
	return vatTotal, breakdown
}

// CalculateQuote computes financial totals for a set of line items.
// Per Dutch/EU accounting rules: VAT is calculated per line, summed, then discount
// is applied proportionally. Optional items get full calculation for transparency
// but are excluded from the grand total.
func CalculateQuote(req transport.QuoteCalculationRequest) transport.QuoteCalculationResponse {
	pricingMode := req.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}
	discountType := req.DiscountType
	if discountType == "" {
		discountType = "percentage"
	}

	var subtotalFloat float64
	vatMap := make(map[int]float64)
	calculatedLines := make([]transport.CalculatedLineItem, 0, len(req.Items))

	for _, item := range req.Items {
		qty := parseQuantityNumber(item.Quantity)
		netUnitPrice := computeLineNetPrice(item.UnitPriceCents, item.TaxRateBps, pricingMode)
		lineSubtotal := qty * netUnitPrice
		lineVat := lineSubtotal * (float64(item.TaxRateBps) / 10000.0)

		calculatedLines = append(calculatedLines, transport.CalculatedLineItem{
			Description:         item.Description,
			Quantity:            item.Quantity,
			UnitPriceCents:      item.UnitPriceCents,
			TaxRateBps:          item.TaxRateBps,
			IsOptional:          item.IsOptional,
			IsSelected:          item.IsSelected,
			TotalBeforeTaxCents: roundCents(lineSubtotal),
			TotalTaxCents:       roundCents(lineVat),
			LineTotalCents:      roundCents(lineSubtotal + lineVat),
		})

		// Include in totals if: non-optional, OR optional AND selected by customer
		if !item.IsOptional || item.IsSelected {
			subtotalFloat += lineSubtotal
			vatMap[item.TaxRateBps] += lineVat
		}
	}

	subtotalCents := roundCents(subtotalFloat)
	discountAmountFloat := computeDiscount(subtotalFloat, discountType, req.DiscountValue)
	discountAmountCents := roundCents(discountAmountFloat)

	// Proportional VAT reduction: if you give 10% off, you owe 10% less VAT
	multiplier := 1.0
	if subtotalFloat > 0 && discountAmountFloat > 0 {
		multiplier = (subtotalFloat - discountAmountFloat) / subtotalFloat
	}

	vatTotal, breakdown := computeVatBreakdown(vatMap, multiplier)
	totalCents := subtotalCents - discountAmountCents + vatTotal

	return transport.QuoteCalculationResponse{
		Lines:               calculatedLines,
		SubtotalCents:       subtotalCents,
		DiscountAmountCents: discountAmountCents,
		VatTotalCents:       vatTotal,
		VatBreakdown:        breakdown,
		TotalCents:          totalCents,
	}
}
