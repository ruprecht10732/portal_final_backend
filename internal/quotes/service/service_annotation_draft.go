package service

import (
	"context"
	"strings"

	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

func (s *Service) SuggestAnnotationReplyDraft(ctx context.Context, quoteID, itemID, tenantID, requesterUserID uuid.UUID) (*transport.SuggestAnnotationReplyDraftResponse, error) {
	if s.replyDrafter == nil {
		return nil, apperr.Internal("quote reply drafts are not configured")
	}

	quote, err := s.repo.GetByID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, err
	}
	if quote.Status != string(transport.QuoteStatusSent) {
		return nil, apperr.Validation("reply drafts are only available for sent quotes")
	}

	item, err := s.repo.GetItemByID(ctx, itemID, quote.ID)
	if err != nil {
		return nil, err
	}

	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, quote.ID)
	if err != nil {
		return nil, err
	}

	thread := buildQuoteAnnotationThread(itemID, annotations)
	if !threadContainsCustomerQuestion(thread) {
		return nil, apperr.Validation("reply drafts require at least one customer question")
	}

	draft, err := s.replyDrafter.SuggestReplyDraft(ctx, SuggestQuoteAnnotationReplyDraftInput{
		OrganizationID:  tenantID,
		RequesterUserID: requesterUserID,
		QuoteID:         quote.ID,
		LeadID:          quote.LeadID,
		LeadServiceID:   quote.LeadServiceID,
		QuoteNumber:     quote.QuoteNumber,
		CustomerName:    buildQuoteDraftCustomerName(quote),
		ItemTitle:       strings.TrimSpace(item.Title),
		ItemDescription: strings.TrimSpace(item.Description),
		Messages:        thread,
	})
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(draft.Text)
	if text == "" {
		return nil, apperr.Internal("quote reply draft returned empty text")
	}

	return &transport.SuggestAnnotationReplyDraftResponse{Text: text}, nil
}

func buildQuoteAnnotationThread(itemID uuid.UUID, annotations []repository.QuoteAnnotation) []QuoteAnnotationReplyDraftMessage {
	thread := make([]QuoteAnnotationReplyDraftMessage, 0)
	for _, annotation := range annotations {
		if annotation.QuoteItemID != itemID {
			continue
		}
		thread = append(thread, QuoteAnnotationReplyDraftMessage{
			AuthorType: annotation.AuthorType,
			Text:       strings.TrimSpace(annotation.Text),
			CreatedAt:  annotation.CreatedAt,
		})
	}
	return thread
}

func threadContainsCustomerQuestion(thread []QuoteAnnotationReplyDraftMessage) bool {
	for _, message := range thread {
		if message.AuthorType == "customer" && strings.TrimSpace(message.Text) != "" {
			return true
		}
	}
	return false
}

func buildQuoteDraftCustomerName(quote *repository.Quote) string {
	if quote == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(ptrStringValue(quote.CustomerFirstName)) + " " + strings.TrimSpace(ptrStringValue(quote.CustomerLastName)))
}
