package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"
	
	"github.com/google/uuid"
)

func generatePublicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func tokenExpiresAt(q *repository.Quote, kind repository.TokenKind) *time.Time {
	if kind == repository.TokenKindPreview {
		return q.PreviewTokenExpAt
	}
	return q.PublicTokenExpAt
}

func isReadOnlyToken(kind repository.TokenKind) bool { return kind == repository.TokenKindPreview }

func (s *Service) resolveToken(ctx context.Context, token string) (*repository.Quote, repository.TokenKind, error) {
	return s.repo.GetByToken(ctx, token)
}

// computeTokenExpiry returns the expiration time for a token, preferring validUntil
// when it is set and in the future; otherwise falling back to defaultPublicTokenTTL.
func computeTokenExpiry(validUntil *time.Time) time.Time {
	now := time.Now()
	expiresAt := now.Add(defaultPublicTokenTTL)
	if validUntil != nil && validUntil.After(now) {
		expiresAt = *validUntil
	}
	return expiresAt
}

func validateSendableQuoteStatus(status string) error {
	if status != string(transport.QuoteStatusDraft) && status != string(transport.QuoteStatusSent) {
		return apperr.BadRequest("only draft or sent quotes can be sent")
	}
	return nil
}

func (s *Service) ensureQuotePublicToken(ctx context.Context, quote *repository.Quote, tenantID uuid.UUID) (string, error) {
	token := strings.TrimSpace(ptrToString(quote.PublicToken))
	if token != "" {
		now := time.Now()
		if quote.PublicTokenExpAt != nil && quote.PublicTokenExpAt.After(now) {
			return token, nil
		}
		// Token exists but has expired — refresh its expiration.
		expiresAt := computeTokenExpiry(quote.ValidUntil)
		if err := s.repo.SetPublicToken(ctx, quote.ID, tenantID, token, expiresAt); err != nil {
			return "", err
		}
		quote.PublicTokenExpAt = &expiresAt
		return token, nil
	}

	if reusedToken, reusedExpAt, reused, err := s.tryReuseVersionChainPublicToken(ctx, quote, tenantID); err != nil {
		return "", err
	} else if reused {
		quote.PublicToken = &reusedToken
		quote.PublicTokenExpAt = reusedExpAt
		return reusedToken, nil
	}

	generatedToken, err := generatePublicToken()
	if err != nil {
		return "", err
	}

	expiresAt := computeTokenExpiry(quote.ValidUntil)
	if err := s.repo.SetPublicToken(ctx, quote.ID, tenantID, generatedToken, expiresAt); err != nil {
		return "", err
	}

	return generatedToken, nil
}

func (s *Service) tryReuseVersionChainPublicToken(ctx context.Context, quote *repository.Quote, tenantID uuid.UUID) (string, *time.Time, bool, error) {
	versionRootID := resolveQuoteVersionRootID(quote)
	if versionRootID == uuid.Nil || versionRootID == quote.ID {
		return "", nil, false, nil
	}
	return s.repo.MoveChainPublicToken(ctx, versionRootID, quote.ID, tenantID)
}

func (s *Service) ensureQuoteStatusSent(ctx context.Context, quoteID, tenantID uuid.UUID, currentStatus string) error {
	if currentStatus == string(transport.QuoteStatusSent) {
		return nil
	}
	return s.repo.UpdateStatus(ctx, quoteID, tenantID, string(transport.QuoteStatusSent))
}

func (s *Service) publishQuoteSentEvent(ctx context.Context, quote *repository.Quote, tenantID, agentID uuid.UUID, token string) {
	if s.eventBus == nil {
		return
	}

	evt := events.QuoteSent{
		BaseEvent:      events.NewBaseEvent(),
		QuoteID:        quote.ID,
		OrganizationID: tenantID,
		LeadID:         quote.LeadID,
		LeadServiceID:  quote.LeadServiceID,
		ISDESubsidy:    quoteSubsidyEventPayload(quote.SubsidyData),
		PublicToken:    token,
		QuoteNumber:    quote.QuoteNumber,
		AgentID:        agentID,
	}

	if s.contacts != nil {
		if contactData, contactErr := s.contacts.GetQuoteContactData(ctx, quote.LeadID, tenantID); contactErr == nil {
			evt.ConsumerEmail = contactData.ConsumerEmail
			evt.ConsumerName = contactData.ConsumerName
			evt.ConsumerPhone = contactData.ConsumerPhone
			evt.OrganizationName = contactData.OrganizationName
		}
	}

	s.eventBus.Publish(ctx, evt)
}

func (s *Service) Send(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, agentID uuid.UUID) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if err := validateSendableQuoteStatus(quote.Status); err != nil {
		return nil, err
	}

	token, err := s.ensureQuotePublicToken(ctx, quote, tenantID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureQuoteStatusSent(ctx, id, tenantID, quote.Status); err != nil {
		return nil, err
	}
	resp, err := s.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	s.publishQuoteSentEvent(ctx, quote, tenantID, agentID, token)
	s.emitTimelineEvent(ctx, TimelineEventParams{LeadID: quote.LeadID, ServiceID: quote.LeadServiceID, OrganizationID: tenantID, ActorType: "User", ActorName: agentID.String(), EventType: "quote_sent", Title: fmt.Sprintf("Quote %s sent", quote.QuoteNumber), Summary: toPtr(fmt.Sprintf(msgTotalFormat, float64(quote.TotalCents)/100)), Metadata: map[string]any{"quoteId": id, "status": "Sent"}})
	return resp, nil
}

func (s *Service) GetPublicQuoteID(ctx context.Context, token string) (uuid.UUID, error) {
	quote, _, err := s.resolveToken(ctx, token)
	if err != nil {
		return uuid.Nil, err
	}
	return quote.ID, nil
}

func (s *Service) GetPublicQuoteStorageMeta(ctx context.Context, token string) (*PublicQuoteStorageMeta, error) {
	quote, _, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	pdfFileKey := ""
	if quote.PDFFileKey != nil {
		pdfFileKey = *quote.PDFFileKey
	}
	return &PublicQuoteStorageMeta{QuoteID: quote.ID, OrgID: quote.OrganizationID, PDFFileKey: pdfFileKey}, nil
}

// InvalidateQuotePDF clears the stored PDF file key so the next download triggers regeneration.
func (s *Service) InvalidateQuotePDF(ctx context.Context, quoteID uuid.UUID) error {
	return s.repo.SetPDFFileKey(ctx, quoteID, "")
}

func (s *Service) GetPreviewLink(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*transport.QuotePreviewLinkResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if quote.PreviewToken != nil && quote.PreviewTokenExpAt != nil && quote.PreviewTokenExpAt.After(now) {
		return &transport.QuotePreviewLinkResponse{Token: *quote.PreviewToken, ExpiresAt: quote.PreviewTokenExpAt}, nil
	}
	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}
	expiresAt := computeTokenExpiry(quote.ValidUntil)
	if err := s.repo.SetPreviewToken(ctx, id, tenantID, token, expiresAt); err != nil {
		return nil, err
	}
	return &transport.QuotePreviewLinkResponse{Token: token, ExpiresAt: &expiresAt}, nil
}
