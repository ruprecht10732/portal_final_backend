package adapters

import (
	"context"
	"errors"

	imapsvc "portal_final_backend/internal/imap/service"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/platform/apperr"
)

type EmailReplySuggesterAdapter struct {
	generator ports.EmailReplyGenerator
}

func NewEmailReplySuggesterAdapter(generator ports.EmailReplyGenerator) *EmailReplySuggesterAdapter {
	return &EmailReplySuggesterAdapter{generator: generator}
}

func (a *EmailReplySuggesterAdapter) SuggestReply(ctx context.Context, input imapsvc.SuggestEmailReplyInput) (string, error) {
	if a == nil || a.generator == nil {
		return "", nil
	}

	mapped := ports.EmailReplyInput{
		OrganizationID:  input.OrganizationID,
		RequesterUserID: input.RequesterUserID,
		LeadID:          input.LeadID,
		LeadServiceID:   input.LeadServiceID,
		CustomerEmail:   input.CustomerEmail,
		CustomerName:    input.CustomerName,
		Subject:         input.Subject,
		MessageBody:     input.MessageBody,
		Feedback:        make([]ports.EmailReplyFeedback, 0, len(input.Feedback)),
		Examples:        make([]ports.EmailReplyExample, 0, len(input.Examples)),
	}

	for _, feedback := range input.Feedback {
		mapped.Feedback = append(mapped.Feedback, ports.EmailReplyFeedback{
			AIReply:    feedback.AIReply,
			HumanReply: feedback.HumanReply,
			CreatedAt:  feedback.CreatedAt,
		})
	}
	for _, example := range input.Examples {
		mapped.Examples = append(mapped.Examples, ports.EmailReplyExample{
			CustomerMessage: example.CustomerMessage,
			Reply:           example.Reply,
			CreatedAt:       example.CreatedAt,
		})
	}

	result, err := a.generator.SuggestEmailReply(ctx, mapped)
	if err != nil {
		if errors.Is(err, ports.ErrEmailReplyLeadContextUnavailable) {
			return "", apperr.Validation("suggest reply is alleen beschikbaar voor e-mails met een gekoppelde lead en actieve dienst")
		}
		return "", err
	}
	return result, nil
}
