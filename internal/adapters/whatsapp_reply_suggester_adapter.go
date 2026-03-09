package adapters

import (
	"context"

	identitysvc "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/leads/ports"
)

type WhatsAppReplySuggesterAdapter struct {
	generator ports.WhatsAppReplyGenerator
}

func NewWhatsAppReplySuggesterAdapter(generator ports.WhatsAppReplyGenerator) *WhatsAppReplySuggesterAdapter {
	return &WhatsAppReplySuggesterAdapter{generator: generator}
}

func (a *WhatsAppReplySuggesterAdapter) SuggestReply(ctx context.Context, input identitysvc.SuggestWhatsAppReplyInput) (string, error) {
	if a == nil || a.generator == nil {
		return "", nil
	}

	mapped := ports.WhatsAppReplyInput{
		OrganizationID: input.OrganizationID,
		LeadID:         input.LeadID,
		ConversationID: input.ConversationID,
		PhoneNumber:    input.PhoneNumber,
		DisplayName:    input.DisplayName,
		Messages:       make([]ports.WhatsAppReplyMessage, 0, len(input.Messages)),
	}

	for _, message := range input.Messages {
		mapped.Messages = append(mapped.Messages, ports.WhatsAppReplyMessage{
			Direction: message.Direction,
			Body:      message.Body,
			CreatedAt: message.CreatedAt,
		})
	}

	return a.generator.SuggestWhatsAppReply(ctx, mapped)
}
