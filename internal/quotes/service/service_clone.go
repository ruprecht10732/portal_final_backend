package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"
	
	"github.com/google/uuid"
)

func (s *Service) Duplicate(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID) (*transport.QuoteResponse, error) {
	return s.cloneQuote(ctx, id, tenantID, actorID, quoteCloneModeDuplicate)
}

func (s *Service) CreateVersion(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID) (*transport.QuoteResponse, error) {
	return s.cloneQuote(ctx, id, tenantID, actorID, quoteCloneModeVersion)
}

type quoteCloneMode string

const (
	quoteCloneModeDuplicate quoteCloneMode = "duplicate"
	quoteCloneModeVersion   quoteCloneMode = "version"
)

type quoteClonePayload struct {
	quote              *repository.Quote
	items              []repository.QuoteItem
	attachments        []repository.QuoteAttachment
	urls               []repository.QuoteURL
	snapshotSourceType string
}

func (s *Service) cloneQuote(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, mode quoteCloneMode) (*transport.QuoteResponse, error) {
	source, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if err := validateCloneSourceStatus(source, mode); err != nil {
		return nil, err
	}

	items, attachments, urls, err := s.loadQuoteCloneData(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	clone, snapshotSourceType, err := s.prepareClonedQuote(ctx, tenantID, actorID, source, mode)
	if err != nil {
		return nil, err
	}

	clonedItems := cloneQuoteItems(clone.ID, tenantID, clone.CreatedAt, items)
	payload := quoteClonePayload{
		quote:              clone,
		items:              clonedItems,
		attachments:        attachments,
		urls:               urls,
		snapshotSourceType: snapshotSourceType,
	}
	if err := s.persistClonedQuote(ctx, tenantID, actorID, payload); err != nil {
		return nil, err
	}
	if mode == quoteCloneModeVersion {
		if err := s.repo.CopyAnnotationsToQuoteVersion(ctx, source.ID, clone.ID, tenantID); err != nil {
			return nil, err
		}
		if err := s.transferPreviewLinkToClone(ctx, source, clone, tenantID); err != nil {
			return nil, err
		}
	}

	s.emitCloneTimelineEvent(ctx, tenantID, actorID, source, clone, mode)

	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, clone.ID)
	if err != nil {
		annotations = nil
	}

	return s.buildResponse(ctx, clone, clonedItems, annotations)
}

func validateCloneSourceStatus(source *repository.Quote, mode quoteCloneMode) error {
	if mode != quoteCloneModeVersion {
		return nil
	}
	return validateVersionSourceStatus(source.Status)
}

func (s *Service) loadQuoteCloneData(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) ([]repository.QuoteItem, []repository.QuoteAttachment, []repository.QuoteURL, error) {
	items, err := s.repo.GetItemsByQuoteID(ctx, id, tenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	attachments, err := s.repo.GetAttachmentsByQuoteID(ctx, id, tenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	urls, err := s.repo.GetURLsByQuoteID(ctx, id, tenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	return items, attachments, urls, nil
}

func (s *Service) prepareClonedQuote(ctx context.Context, tenantID uuid.UUID, actorID uuid.UUID, source *repository.Quote, mode quoteCloneMode) (*repository.Quote, string, error) {
	quoteNumber, err := s.repo.NextQuoteNumber(ctx, tenantID)
	if err != nil {
		return nil, "", fmt.Errorf("generate quote number: %w", err)
	}

	now := time.Now()
	validUntil := cloneQuoteValidUntil(now, source.ValidUntil)
	if validUntil == nil {
		_, validDays := s.resolveEffectiveQuoteTerms(ctx, tenantID, source.LeadID, source.LeadServiceID)
		if validDays > 0 {
			expiresAt := now.AddDate(0, 0, validDays)
			validUntil = &expiresAt
		}
	}

	clone := &repository.Quote{
		ID:                  uuid.New(),
		OrganizationID:      tenantID,
		LeadID:              source.LeadID,
		LeadServiceID:       source.LeadServiceID,
		CreatedByID:         &actorID,
		QuoteNumber:         quoteNumber,
		Status:              string(transport.QuoteStatusDraft),
		PricingMode:         source.PricingMode,
		DiscountType:        source.DiscountType,
		DiscountValue:       source.DiscountValue,
		SubtotalCents:       source.SubtotalCents,
		DiscountAmountCents: source.DiscountAmountCents,
		TaxTotalCents:       source.TaxTotalCents,
		TotalCents:          source.TotalCents,
		ValidUntil:          validUntil,
		Notes:               source.Notes,
		SubsidyData:         append([]byte(nil), source.SubsidyData...),
		FinancingDisclaimer: source.FinancingDisclaimer,
		CreatedAt:           now,
		UpdatedAt:           now,
		VersionNumber:       1,
	}

	snapshotSourceType := "manual_duplicate"
	if mode == quoteCloneModeVersion {
		snapshotSourceType = "manual_version"
		versionRootID, nextVersion, versionErr := resolveQuoteVersionLineage(ctx, s.repo, source, tenantID)
		if versionErr != nil {
			return nil, "", versionErr
		}
		clone.PreviousVersionQuoteID = &source.ID
		clone.VersionRootQuoteID = &versionRootID
		clone.VersionNumber = nextVersion
	} else {
		clone.DuplicatedFromQuoteID = &source.ID
	}
	return clone, snapshotSourceType, nil
}

func (s *Service) persistClonedQuote(ctx context.Context, tenantID uuid.UUID, actorID uuid.UUID, payload quoteClonePayload) error {
	if err := s.repo.CreateWithItems(ctx, payload.quote, payload.items, &repository.QuotePricingSnapshot{
		QuoteID:             payload.quote.ID,
		OrganizationID:      tenantID,
		LeadID:              payload.quote.LeadID,
		LeadServiceID:       payload.quote.LeadServiceID,
		SourceType:          payload.snapshotSourceType,
		PricingMode:         payload.quote.PricingMode,
		DiscountType:        payload.quote.DiscountType,
		DiscountValue:       payload.quote.DiscountValue,
		SubtotalCents:       payload.quote.SubtotalCents,
		DiscountAmountCents: payload.quote.DiscountAmountCents,
		TaxTotalCents:       payload.quote.TaxTotalCents,
		TotalCents:          payload.quote.TotalCents,
		CreatedByActor:      "user",
		CreatedByUserID:     &actorID,
	}); err != nil {
		return err
	}

	if err := s.saveAttachments(ctx, payload.quote.ID, tenantID, cloneAttachmentRequests(payload.attachments)); err != nil {
		return fmt.Errorf(errSaveAttachmentsFmt, err)
	}
	if err := s.saveURLs(ctx, payload.quote.ID, tenantID, cloneURLRequests(payload.urls)); err != nil {
		return fmt.Errorf(errSaveURLsFmt, err)
	}
	return nil
}

func (s *Service) emitCloneTimelineEvent(ctx context.Context, tenantID uuid.UUID, actorID uuid.UUID, source *repository.Quote, clone *repository.Quote, mode quoteCloneMode) {
	eventType := "quote_duplicated"
	title := fmt.Sprintf("Quote %s duplicated from %s", clone.QuoteNumber, source.QuoteNumber)
	metadata := map[string]any{
		"quoteId":           clone.ID,
		"sourceQuoteId":     source.ID,
		"sourceQuoteNumber": source.QuoteNumber,
		"status":            clone.Status,
	}
	if mode == quoteCloneModeVersion {
		eventType = "quote_version_created"
		title = fmt.Sprintf("Quote %s v%d created from %s", clone.QuoteNumber, clone.VersionNumber, source.QuoteNumber)
		metadata["versionNumber"] = clone.VersionNumber
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         clone.LeadID,
		ServiceID:      clone.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      eventType,
		Title:          title,
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(clone.TotalCents)/100)),
		Metadata:       metadata,
	})
}

func validateVersionSourceStatus(status string) error {
	if status != string(transport.QuoteStatusDraft) && status != string(transport.QuoteStatusSent) {
		return apperr.BadRequest("new version can only be created from draft or sent quotes")
	}
	return nil
}

func (s *Service) transferPreviewLinkToClone(ctx context.Context, source *repository.Quote, clone *repository.Quote, tenantID uuid.UUID) error {
	token := strings.TrimSpace(ptrToString(source.PreviewToken))
	if token == "" {
		return nil
	}
	if err := s.repo.MovePreviewToken(ctx, source.ID, clone.ID, tenantID); err != nil {
		return err
	}
	clone.PreviewToken = source.PreviewToken
	clone.PreviewTokenExpAt = source.PreviewTokenExpAt
	return nil
}

func resolveQuoteVersionRootID(q *repository.Quote) uuid.UUID {
	if q.VersionRootQuoteID != nil && *q.VersionRootQuoteID != uuid.Nil {
		return *q.VersionRootQuoteID
	}
	return q.ID
}

func cloneQuoteValidUntil(now time.Time, sourceValidUntil *time.Time) *time.Time {
	if sourceValidUntil == nil {
		return nil
	}
	if sourceValidUntil.Before(now) {
		return nil
	}
	copyValue := *sourceValidUntil
	return &copyValue
}

func resolveQuoteVersionLineage(ctx context.Context, repo *repository.Repository, source *repository.Quote, tenantID uuid.UUID) (uuid.UUID, int, error) {
	versionRootID := source.ID
	if source.VersionRootQuoteID != nil {
		versionRootID = *source.VersionRootQuoteID
	}
	nextVersion, err := repo.NextQuoteVersionNumber(ctx, tenantID, versionRootID)
	if err != nil {
		return uuid.Nil, 0, err
	}
	return versionRootID, nextVersion, nil
}

func cloneQuoteItems(quoteID, tenantID uuid.UUID, createdAt time.Time, items []repository.QuoteItem) []repository.QuoteItem {
	cloned := make([]repository.QuoteItem, len(items))
	for i, item := range items {
		cloned[i] = repository.QuoteItem{
			ID:               uuid.New(),
			QuoteID:          quoteID,
			OrganizationID:   tenantID,
			Title:            item.Title,
			Description:      item.Description,
			Quantity:         item.Quantity,
			QuantityNumeric:  item.QuantityNumeric,
			UnitPriceCents:   item.UnitPriceCents,
			TaxRateBps:       item.TaxRateBps,
			IsOptional:       item.IsOptional,
			IsSelected:       item.IsSelected,
			SortOrder:        item.SortOrder,
			CatalogProductID: item.CatalogProductID,
			CreatedAt:        createdAt,
		}
	}
	return cloned
}

func cloneAttachmentRequests(attachments []repository.QuoteAttachment) []transport.QuoteAttachmentRequest {
	result := make([]transport.QuoteAttachmentRequest, len(attachments))
	for i, attachment := range attachments {
		result[i] = transport.QuoteAttachmentRequest{
			Filename:         attachment.Filename,
			FileKey:          attachment.FileKey,
			Source:           attachment.Source,
			CatalogProductID: attachment.CatalogProductID,
			Enabled:          attachment.Enabled,
			SortOrder:        attachment.SortOrder,
		}
	}
	return result
}

func cloneURLRequests(urls []repository.QuoteURL) []transport.QuoteURLRequest {
	result := make([]transport.QuoteURLRequest, len(urls))
	for i, quoteURL := range urls {
		result[i] = transport.QuoteURLRequest{
			Label:            quoteURL.Label,
			Href:             quoteURL.Href,
			CatalogProductID: quoteURL.CatalogProductID,
		}
	}
	return result
}
