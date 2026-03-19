package adapters

import (
	"context"
	"fmt"
	"strings"

	"portal_final_backend/internal/leads/ports"
	quotesvc "portal_final_backend/internal/quotes/service"
)

type QuoteAnnotationReplyDraftAdapter struct {
	generator ports.WhatsAppReplyGenerator
}

func NewQuoteAnnotationReplyDraftAdapter(generator ports.WhatsAppReplyGenerator) *QuoteAnnotationReplyDraftAdapter {
	return &QuoteAnnotationReplyDraftAdapter{generator: generator}
}

func (a *QuoteAnnotationReplyDraftAdapter) SuggestReplyDraft(ctx context.Context, input quotesvc.SuggestQuoteAnnotationReplyDraftInput) (quotesvc.QuoteAnnotationReplyDraft, error) {
	if a == nil || a.generator == nil {
		return quotesvc.QuoteAnnotationReplyDraft{}, nil
	}

	leadID := input.LeadID
	messages := make([]ports.WhatsAppReplyMessage, 0, len(input.Messages))
	for _, message := range input.Messages {
		messages = append(messages, ports.WhatsAppReplyMessage{
			Direction: mapQuoteAnnotationDirection(message.AuthorType),
			Body:      message.Text,
			CreatedAt: message.CreatedAt,
		})
	}

	result, err := a.generator.SuggestWhatsAppReply(ctx, ports.WhatsAppReplyInput{
		OrganizationID:  input.OrganizationID,
		RequesterUserID: input.RequesterUserID,
		LeadID:          &leadID,
		ConversationID:  input.QuoteID,
		Scenario:        ports.ReplySuggestionScenarioGeneric,
		ScenarioNotes:   buildQuoteAnnotationScenarioNotes(input),
		DisplayName:     input.CustomerName,
		Messages:        messages,
	})
	if err != nil {
		return quotesvc.QuoteAnnotationReplyDraft{}, err
	}

	return quotesvc.QuoteAnnotationReplyDraft{Text: strings.TrimSpace(result.Text)}, nil
}

func mapQuoteAnnotationDirection(authorType string) string {
	if strings.EqualFold(authorType, "agent") {
		return "outbound"
	}
	return "inbound"
}

func buildQuoteAnnotationScenarioNotes(input quotesvc.SuggestQuoteAnnotationReplyDraftInput) string {
	contextParts := make([]string, 0, 4)
	if strings.TrimSpace(input.QuoteNumber) != "" {
		contextParts = append(contextParts, fmt.Sprintf("Offerte: %s", strings.TrimSpace(input.QuoteNumber)))
	}
	itemLabel := strings.TrimSpace(input.ItemTitle)
	if itemLabel == "" {
		itemLabel = strings.TrimSpace(input.ItemDescription)
	}
	if itemLabel != "" {
		contextParts = append(contextParts, fmt.Sprintf("Offertepost: %s", itemLabel))
	}
	contextParts = append(contextParts,
		"Dit is een interne conceptreactie op een klantvraag binnen een offerte.",
		"Schrijf alleen de antwoordtekst die een medewerker direct bij de offertevraag kan plaatsen.",
		"Blijf concreet, vriendelijk en kort. Gebruik geen onderwerpregel en voeg geen extra kanaaltekst toe.",
	)
	return strings.Join(contextParts, "\n")
}
