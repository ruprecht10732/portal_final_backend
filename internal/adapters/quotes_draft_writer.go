package adapters

import (
	"context"
	"fmt"

	"portal_final_backend/internal/leads/ports"
	quotesvc "portal_final_backend/internal/quotes/service"
)

// QuotesDraftWriter adapts the quotes service for the leads domain.
// It implements ports.QuoteDrafter by delegating to quotesvc.Service.DraftQuote.
type QuotesDraftWriter struct {
	svc *quotesvc.Service
}

// NewQuotesDraftWriter creates a new quote drafter adapter.
func NewQuotesDraftWriter(svc *quotesvc.Service) *QuotesDraftWriter {
	return &QuotesDraftWriter{svc: svc}
}

// DraftQuote translates the leads-domain DraftQuoteParams to the quotes-domain
// DraftQuoteParams and delegates to the quotes service.
func (a *QuotesDraftWriter) DraftQuote(ctx context.Context, params ports.DraftQuoteParams) (*ports.DraftQuoteResult, error) {
	svcItems := make([]quotesvc.DraftQuoteItemParams, len(params.Items))
	for i, it := range params.Items {
		svcItems[i] = quotesvc.DraftQuoteItemParams{
			Description:      it.Description,
			Quantity:         it.Quantity,
			UnitPriceCents:   it.UnitPriceCents,
			TaxRateBps:       it.TaxRateBps,
			IsOptional:       it.IsOptional,
			CatalogProductID: it.CatalogProductID,
		}
	}

	result, err := a.svc.DraftQuote(ctx, quotesvc.DraftQuoteParams{
		LeadID:         params.LeadID,
		LeadServiceID:  params.LeadServiceID,
		OrganizationID: params.OrganizationID,
		CreatedByID:    params.CreatedByID,
		Notes:          params.Notes,
		Items:          svcItems,
	})
	if err != nil {
		return nil, fmt.Errorf("quotes draft adapter: %w", err)
	}

	return &ports.DraftQuoteResult{
		QuoteID:     result.QuoteID,
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

// Compile-time check that QuotesDraftWriter implements ports.QuoteDrafter.
var _ ports.QuoteDrafter = (*QuotesDraftWriter)(nil)
