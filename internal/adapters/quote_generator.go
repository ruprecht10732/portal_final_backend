package adapters

import (
	"context"
	"fmt"

	"portal_final_backend/internal/leads/agent"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuoteGeneratorAdapter adapts the leads module's QuoteGenerator agent to the
// quotes service's QuotePromptGenerator interface, following the ACL pattern.
type QuoteGeneratorAdapter struct {
	quoteGenerator agent.QuoteGenerator
}

// NewQuoteGeneratorAdapter creates a new QuoteGeneratorAdapter wrapping a quote generator.
func NewQuoteGeneratorAdapter(generator agent.QuoteGenerator) *QuoteGeneratorAdapter {
	return &QuoteGeneratorAdapter{quoteGenerator: generator}
}

// GenerateFromPrompt calls the leads module's QuoteGenerator agent and maps the result.
func (a *QuoteGeneratorAdapter) GenerateFromPrompt(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, prompt string, existingQuoteID *uuid.UUID, force bool) (*quotesvc.GenerateQuoteResult, error) {
	result, err := a.quoteGenerator.Generate(ctx, leadID, serviceID, tenantID, prompt, existingQuoteID, force)
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
