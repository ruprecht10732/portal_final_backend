package adapters

import (
	"context"
	"fmt"

	"portal_final_backend/internal/leads"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuoteGeneratorAdapter adapts the leads module's QuoteGenerator agent to the
// quotes service's QuotePromptGenerator interface, following the ACL pattern.
type QuoteGeneratorAdapter struct {
	leadsModule *leads.Module
}

// NewQuoteGeneratorAdapter creates a new QuoteGeneratorAdapter wrapping the leads module.
func NewQuoteGeneratorAdapter(m *leads.Module) *QuoteGeneratorAdapter {
	return &QuoteGeneratorAdapter{leadsModule: m}
}

// GenerateFromPrompt calls the leads module's QuoteGenerator agent and maps the result.
func (a *QuoteGeneratorAdapter) GenerateFromPrompt(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) (*quotesvc.GenerateQuoteResult, error) {
	result, err := a.leadsModule.GenerateQuoteFromPrompt(ctx, leadID, serviceID, tenantID, prompt, existingQuoteID)
	if err != nil {
		return nil, fmt.Errorf("quote generation failed: %w", err)
	}

	return &quotesvc.GenerateQuoteResult{
		QuoteID:     result.QuoteID,
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

// Compile-time check that QuoteGeneratorAdapter implements QuotePromptGenerator.
var _ quotesvc.QuotePromptGenerator = (*QuoteGeneratorAdapter)(nil)
