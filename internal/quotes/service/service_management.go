package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/events"
	leadrepo "portal_final_backend/internal/leads/repository"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	errSaveAttachmentsFmt = "save attachments: %w"
	errSaveURLsFmt        = "save urls: %w"
	extraWorkTitle        = "Extra work"
	extraWorkFallbackDesc = "Additional work completed during fulfillment"
	defaultTaxRateBps    = 2100
)

func inferExtraWorkTaxRate(items []repository.QuoteItem) int {
	for _, item := range items {
		if !item.IsOptional || item.IsSelected {
			return item.TaxRateBps
		}
	}
	if len(items) > 0 {
		return items[0].TaxRateBps
	}
	return defaultTaxRateBps
}

func buildExtraWorkItemRequest(amountCents int64, notes *string, items []repository.QuoteItem) transport.QuoteItemRequest {
	description := extraWorkFallbackDesc
	if notes != nil {
		trimmed := strings.TrimSpace(*notes)
		if trimmed != "" {
			description = trimmed
		}
	}
	return transport.QuoteItemRequest{
		Title:          extraWorkTitle,
		Description:    description,
		Quantity:       "1",
		UnitPriceCents: amountCents,
		TaxRateBps:     inferExtraWorkTaxRate(items),
		IsOptional:     false,
		IsSelected:     true,
	}
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, actorID uuid.UUID, req transport.CreateQuoteRequest) (*transport.QuoteResponse, error) {
	quoteNumber, err := s.repo.NextQuoteNumber(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("generate quote number: %w", err)
	}

	pricingMode := req.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}
	discountType := req.DiscountType
	if discountType == "" {
		discountType = "percentage"
	}

	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: req.Items, PricingMode: pricingMode, DiscountType: discountType, DiscountValue: req.DiscountValue})
	now := time.Now()

	validUntil := req.ValidUntil
	if validUntil == nil {
		_, validDays := s.resolveEffectiveQuoteTerms(ctx, tenantID, req.LeadID, req.LeadServiceID)
		if validDays > 0 {
			exp := now.AddDate(0, 0, validDays)
			validUntil = &exp
		}
	}

	quote := repository.Quote{
		ID:                  uuid.New(),
		OrganizationID:      tenantID,
		LeadID:              req.LeadID,
		LeadServiceID:       req.LeadServiceID,
		VersionNumber:       1,
		CreatedByID:         &actorID,
		QuoteNumber:         quoteNumber,
		Status:              string(transport.QuoteStatusDraft),
		PricingMode:         pricingMode,
		DiscountType:        discountType,
		DiscountValue:       req.DiscountValue,
		SubtotalCents:       calc.SubtotalCents,
		DiscountAmountCents: calc.DiscountAmountCents,
		TaxTotalCents:       calc.VatTotalCents,
		TotalCents:          calc.TotalCents,
		ValidUntil:          validUntil,
		Notes:               nilIfEmpty(req.Notes),
		FinancingDisclaimer: req.FinancingDisclaimer,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	items := make([]repository.QuoteItem, len(req.Items))
	for i, it := range req.Items {
		selected := true
		if it.IsOptional {
			selected = it.IsSelected
		}
		quantity := normalizeQuantityString(it.Quantity)
		items[i] = repository.QuoteItem{
			ID:               uuid.New(),
			QuoteID:          quote.ID,
			OrganizationID:   tenantID,
			Description:      it.Description,
			Quantity:         quantity,
			QuantityNumeric:  parseQuantityNumber(quantity),
			UnitPriceCents:   it.UnitPriceCents,
			TaxRateBps:       it.TaxRateBps,
			IsOptional:       it.IsOptional,
			IsSelected:       selected,
			SortOrder:        i,
			CatalogProductID: it.CatalogProductID,
			CreatedAt:        now,
		}
	}

	if err := s.repo.CreateWithItems(ctx, &quote, items, &repository.QuotePricingSnapshot{
		QuoteID:             quote.ID,
		OrganizationID:      tenantID,
		LeadID:              quote.LeadID,
		LeadServiceID:       quote.LeadServiceID,
		SourceType:          "manual_create",
		PricingMode:         quote.PricingMode,
		DiscountType:        quote.DiscountType,
		DiscountValue:       quote.DiscountValue,
		SubtotalCents:       quote.SubtotalCents,
		DiscountAmountCents: quote.DiscountAmountCents,
		TaxTotalCents:       quote.TaxTotalCents,
		TotalCents:          quote.TotalCents,
		CreatedByActor:      "user",
		CreatedByUserID:     &actorID,
	}); err != nil {
		return nil, err
	}
	if err := s.saveAttachments(ctx, quote.ID, tenantID, req.Attachments); err != nil {
		return nil, fmt.Errorf(errSaveAttachmentsFmt, err)
	}
	if err := s.saveURLs(ctx, quote.ID, tenantID, req.URLs); err != nil {
		return nil, fmt.Errorf(errSaveURLsFmt, err)
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      "quote_created",
		Title:          fmt.Sprintf("Quote %s created", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(quote.TotalCents)/100)),
		Metadata:       map[string]any{"quoteId": quote.ID, "status": quote.Status},
	})

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteCreated{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        quote.ID,
			OrganizationID: tenantID,
			LeadID:         quote.LeadID,
			LeadServiceID:  quote.LeadServiceID,
			QuoteNumber:    quote.QuoteNumber,
			ActorID:        &actorID,
		})
	}

	return s.buildResponse(ctx, &quote, items)
}

func (s *Service) Duplicate(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID) (*transport.QuoteResponse, error) {
	return s.cloneQuote(ctx, id, tenantID, actorID, quoteCloneModeDuplicate)
}

func (s *Service) CreateVersion(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID) (*transport.QuoteResponse, error) {
	return s.cloneQuote(ctx, id, tenantID, actorID, quoteCloneModeVersion)
}

func applyQuoteUpdates(quote *repository.Quote, req transport.UpdateQuoteRequest) {
	if req.PricingMode != nil {
		quote.PricingMode = *req.PricingMode
	}
	if req.DiscountType != nil {
		quote.DiscountType = *req.DiscountType
	}
	if req.DiscountValue != nil {
		quote.DiscountValue = *req.DiscountValue
	}
	if req.ValidUntil != nil {
		quote.ValidUntil = req.ValidUntil
	}
	if req.Notes != nil {
		quote.Notes = req.Notes
	}
	if req.FinancingDisclaimer != nil {
		quote.FinancingDisclaimer = *req.FinancingDisclaimer
	}
}

func buildItemsFromRequest(quoteID, tenantID uuid.UUID, items []transport.QuoteItemRequest) []repository.QuoteItem {
	now := time.Now()
	result := make([]repository.QuoteItem, len(items))
	for i, it := range items {
		selected := true
		if it.IsOptional {
			selected = it.IsSelected
		}
		quantity := normalizeQuantityString(it.Quantity)
		result[i] = repository.QuoteItem{ID: uuid.New(), QuoteID: quoteID, OrganizationID: tenantID, Title: it.Title, Description: it.Description, Quantity: quantity, QuantityNumeric: parseQuantityNumber(quantity), UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: selected, SortOrder: i, CatalogProductID: it.CatalogProductID, CreatedAt: now}
	}
	return result
}

func toItemRequests(items []repository.QuoteItem) []transport.QuoteItemRequest {
	reqs := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		reqs[i] = transport.QuoteItemRequest{Title: it.Title, Description: it.Description, Quantity: normalizeQuantityString(it.Quantity), UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: it.IsSelected, CatalogProductID: it.CatalogProductID}
	}
	return reqs
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, req transport.UpdateQuoteRequest) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	applyQuoteUpdates(quote, req)

	var items []repository.QuoteItem
	if req.Items != nil {
		items = buildItemsFromRequest(quote.ID, tenantID, *req.Items)
	} else {
		items, err = s.repo.GetItemsByQuoteID(ctx, id, tenantID)
		if err != nil {
			return nil, err
		}
	}

	itemReqs := toItemRequests(items)
	if req.Items != nil {
		itemReqs = *req.Items
	}
	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: itemReqs, PricingMode: quote.PricingMode, DiscountType: quote.DiscountType, DiscountValue: quote.DiscountValue})
	quote.SubtotalCents = calc.SubtotalCents
	quote.DiscountAmountCents = calc.DiscountAmountCents
	quote.TaxTotalCents = calc.VatTotalCents
	quote.TotalCents = calc.TotalCents
	quote.UpdatedAt = time.Now()

	if err := s.repo.UpdateWithItems(ctx, quote, items, req.Items != nil, &repository.QuotePricingSnapshot{
		QuoteID:             quote.ID,
		OrganizationID:      tenantID,
		LeadID:              quote.LeadID,
		LeadServiceID:       quote.LeadServiceID,
		SourceType:          "manual_update",
		PricingMode:         quote.PricingMode,
		DiscountType:        quote.DiscountType,
		DiscountValue:       quote.DiscountValue,
		SubtotalCents:       quote.SubtotalCents,
		DiscountAmountCents: quote.DiscountAmountCents,
		TaxTotalCents:       quote.TaxTotalCents,
		TotalCents:          quote.TotalCents,
		CreatedByActor:      "user",
		CreatedByUserID:     &actorID,
	}); err != nil {
		return nil, err
	}
	if req.Attachments != nil {
		if err := s.saveAttachments(ctx, quote.ID, tenantID, *req.Attachments); err != nil {
			return nil, fmt.Errorf(errSaveAttachmentsFmt, err)
		}
	}
	if req.URLs != nil {
		if err := s.saveURLs(ctx, quote.ID, tenantID, *req.URLs); err != nil {
			return nil, fmt.Errorf(errSaveURLsFmt, err)
		}
	}
	if err := s.invalidatePDFOnAssetUpdates(ctx, quote, req); err != nil {
		return nil, err
	}

	return s.buildResponse(ctx, quote, items)
}

func (s *Service) invalidatePDFOnAssetUpdates(ctx context.Context, quote *repository.Quote, req transport.UpdateQuoteRequest) error {
	if req.Attachments == nil && req.URLs == nil {
		return nil
	}
	if quote.PDFFileKey == nil || strings.TrimSpace(*quote.PDFFileKey) == "" {
		return nil
	}
	if err := s.repo.SetPDFFileKey(ctx, quote.ID, ""); err != nil {
		return fmt.Errorf("invalidate quote pdf: %w", err)
	}
	emptyKey := ""
	quote.PDFFileKey = &emptyKey
	return nil
}

func (s *Service) AddExtraWorkToQuote(ctx context.Context, quoteID uuid.UUID, organizationID uuid.UUID, actorID uuid.UUID, amountCents int64, notes *string) error {
	if amountCents <= 0 {
		return nil
	}

	quote, err := s.repo.GetByID(ctx, quoteID, organizationID)
	if err != nil {
		return err
	}

	items, err := s.repo.GetItemsByQuoteID(ctx, quoteID, organizationID)
	if err != nil {
		return err
	}

	itemReqs := toItemRequests(items)
	itemReqs = append(itemReqs, buildExtraWorkItemRequest(amountCents, notes, items))
	updatedItems := buildItemsFromRequest(quote.ID, organizationID, itemReqs)
	calc := CalculateQuote(transport.QuoteCalculationRequest{
		Items:         itemReqs,
		PricingMode:   quote.PricingMode,
		DiscountType:  quote.DiscountType,
		DiscountValue: quote.DiscountValue,
	})

	quote.SubtotalCents = calc.SubtotalCents
	quote.DiscountAmountCents = calc.DiscountAmountCents
	quote.TaxTotalCents = calc.VatTotalCents
	quote.TotalCents = calc.TotalCents
	quote.UpdatedAt = time.Now()

	return s.repo.UpdateWithItems(ctx, quote, updatedItems, true, &repository.QuotePricingSnapshot{
		QuoteID:             quote.ID,
		OrganizationID:      organizationID,
		LeadID:              quote.LeadID,
		LeadServiceID:       quote.LeadServiceID,
		SourceType:          "lead_completion_extra_work",
		PricingMode:         quote.PricingMode,
		DiscountType:        quote.DiscountType,
		DiscountValue:       quote.DiscountValue,
		ExtraCostsCents:     &amountCents,
		SubtotalCents:       quote.SubtotalCents,
		DiscountAmountCents: quote.DiscountAmountCents,
		TaxTotalCents:       quote.TaxTotalCents,
		TotalCents:          quote.TotalCents,
		Notes:               notes,
		CreatedByActor:      "user",
		CreatedByUserID:     &actorID,
	})
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.GetItemsByQuoteID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, id)
	if err != nil {
		annotations = nil
	}
	return s.buildResponse(ctx, quote, items, annotations)
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

	s.emitCloneTimelineEvent(ctx, tenantID, actorID, source, clone, mode)

	return s.buildResponse(ctx, clone, clonedItems)
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
	if status != string(transport.QuoteStatusAccepted) && status != string(transport.QuoteStatusRejected) {
		return apperr.BadRequest("new version can only be created from accepted or rejected quotes")
	}
	return nil
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

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, req transport.ListQuotesRequest) (*transport.QuoteListResponse, error) {
	params := repository.ListParams{OrganizationID: tenantID, Status: nilIfEmpty(req.Status), Search: req.Search, SortBy: req.SortBy, SortOrder: req.SortOrder, Page: max(req.Page, 1), PageSize: clampPageSize(req.PageSize)}
	createdFrom, createdTo, err := parseDateRange(req.CreatedAtFrom, req.CreatedAtTo, "createdAtFrom", "createdAtTo")
	if err != nil {
		return nil, err
	}
	validFrom, validTo, err := parseDateRange(req.ValidUntilFrom, req.ValidUntilTo, "validUntilFrom", "validUntilTo")
	if err != nil {
		return nil, err
	}
	totalFrom, totalTo, err := parseInt64Range(req.TotalFrom, req.TotalTo, "totalFrom", "totalTo")
	if err != nil {
		return nil, err
	}
	params.CreatedAtFrom = createdFrom
	params.CreatedAtTo = createdTo
	params.ValidUntilFrom = validFrom
	params.ValidUntilTo = validTo
	params.TotalFrom = totalFrom
	params.TotalTo = totalTo
	if req.LeadID != "" {
		parsed, parseErr := uuid.Parse(req.LeadID)
		if parseErr != nil {
			return nil, apperr.BadRequest("invalid leadId format")
		}
		params.LeadID = &parsed
	}

	result, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	quoteIDs := make([]uuid.UUID, 0, len(result.Items))
	for _, q := range result.Items {
		quoteIDs = append(quoteIDs, q.ID)
	}

	itemsByQuoteID, err := s.repo.GetItemsByQuoteIDs(ctx, tenantID, quoteIDs)
	if err != nil {
		return nil, err
	}

	items := make([]transport.QuoteResponse, len(result.Items))
	for i, q := range result.Items {
		mapped, mapErr := s.buildResponse(ctx, &q, itemsByQuoteID[q.ID])
		if mapErr != nil {
			return nil, mapErr
		}
		items[i] = *mapped
	}

	return &transport.QuoteListResponse{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}, nil
}

func (s *Service) ListPendingApprovals(ctx context.Context, tenantID uuid.UUID, req transport.ListPendingApprovalsRequest) (*transport.PendingApprovalsResponse, error) {
	result, err := s.repo.ListPendingApprovals(ctx, tenantID, max(req.Page, 1), clampPageSize(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]transport.PendingApprovalItem, len(result.Items))
	for i, row := range result.Items {
		consumerName := strings.TrimSpace(strings.TrimSpace(ptrToString(row.ConsumerFirstName)) + " " + strings.TrimSpace(ptrToString(row.ConsumerLastName)))
		if consumerName == "" {
			consumerName = row.QuoteNumber
		}
		items[i] = transport.PendingApprovalItem{QuoteID: row.QuoteID, LeadID: row.LeadID, QuoteNumber: row.QuoteNumber, ConsumerName: consumerName, TotalCents: row.TotalCents, ConfidenceScore: row.LeadScore, CreatedAt: row.CreatedAt}
	}
	return &transport.PendingApprovalsResponse{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}, nil
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, status transport.QuoteStatus) (*transport.QuoteResponse, error) {
	current, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	oldStatus := current.Status
	if oldStatus == string(status) {
		return s.GetByID(ctx, id, tenantID)
	}

	if err := s.repo.UpdateStatus(ctx, id, tenantID, string(status)); err != nil {
		return nil, err
	}
	resp, err := s.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	s.emitTimelineEvent(ctx, TimelineEventParams{LeadID: resp.LeadID, ServiceID: resp.LeadServiceID, OrganizationID: tenantID, ActorType: "User", ActorName: actorID.String(), EventType: "quote_status_changed", Title: fmt.Sprintf("Quote %s -> %s", resp.QuoteNumber, string(status)), Summary: toPtr(fmt.Sprintf(msgTotalFormat, float64(resp.TotalCents)/100)), Metadata: map[string]any{"quoteId": resp.ID, "status": string(status)}})

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteStatusChanged{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        resp.ID,
			OrganizationID: tenantID,
			LeadID:         resp.LeadID,
			LeadServiceID:  resp.LeadServiceID,
			QuoteNumber:    resp.QuoteNumber,
			OldStatus:      oldStatus,
			NewStatus:      string(status),
			ActorID:        actorID,
		})
	}

	return resp, nil
}

func (s *Service) SetLeadServiceID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, leadServiceID uuid.UUID) (*transport.QuoteResponse, error) {
	current, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if current.LeadServiceID != nil && *current.LeadServiceID == leadServiceID {
		return s.GetByID(ctx, id, tenantID)
	}

	if err := s.repo.SetLeadServiceID(ctx, id, tenantID, leadServiceID); err != nil {
		return nil, err
	}

	resp, err := s.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         resp.LeadID,
		ServiceID:      resp.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      "quote_service_linked",
		Title:          fmt.Sprintf("Quote %s linked to service", resp.QuoteNumber),
		Metadata:       map[string]any{"quoteId": resp.ID, "leadServiceId": leadServiceID},
	})

	return resp, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID) error {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id, tenantID); err != nil {
		return err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteDeleted{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        id,
			OrganizationID: tenantID,
			LeadID:         quote.LeadID,
			LeadServiceID:  quote.LeadServiceID,
			QuoteNumber:    quote.QuoteNumber,
			ActorID:        &actorID,
		})
	}

	return nil
}

func (s *Service) TransferToOrganization(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, destinationTenantID uuid.UUID, actorID uuid.UUID) (*transport.TransferQuoteResponse, error) {
	if tenantID == destinationTenantID {
		return nil, apperr.Validation("destination organization must differ from source organization")
	}
	if s.leadCreator == nil || s.leadRepo == nil {
		return nil, apperr.Internal("quote transfer dependencies are not configured")
	}

	context, err := s.loadQuoteTransferContext(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	createdLead, err := s.createTransferredQuoteLead(ctx, context, destinationTenantID)
	if err != nil {
		return nil, err
	}

	createdQuote, err := s.Create(ctx, destinationTenantID, actorID, buildTransferredQuoteRequest(context, createdLead))
	if err != nil {
		return nil, err
	}
	s.recordQuoteTransferAudit(ctx, quoteTransferAuditParams{
		quoteID:                context.sourceQuote.ID,
		leadID:                 context.sourceQuote.LeadID,
		serviceID:              nil,
		tenantID:               tenantID,
		relatedOrganizationID:  destinationTenantID,
		actorID:                actorID,
		action:                 "transferred_out",
		message:                "Quote transferred to another organization",
	})
	s.recordQuoteTransferAudit(ctx, quoteTransferAuditParams{
		quoteID:                createdQuote.ID,
		leadID:                 createdQuote.LeadID,
		serviceID:              createdQuote.LeadServiceID,
		tenantID:               destinationTenantID,
		relatedOrganizationID:  tenantID,
		actorID:                actorID,
		action:                 "transferred_in",
		message:                "Quote received from another organization",
	})

	if err := s.Delete(ctx, id, tenantID, actorID); err != nil {
		return nil, err
	}

	cleanupPlan := planQuoteTransferSourceCleanup(context.sourceLead.ID, context.sourceService.ID, context.sourceServices)
	if cleanupPlan.DeleteLead {
		if err := s.leadRepo.Delete(ctx, context.sourceLead.ID, tenantID); err != nil {
			return nil, err
		}
	} else if cleanupPlan.ServiceID != nil {
		if err := s.leadRepo.DeleteLeadService(ctx, *cleanupPlan.ServiceID, tenantID); err != nil {
			return nil, err
		}
	}

	return &transport.TransferQuoteResponse{
		Quote:                     *createdQuote,
		DestinationLeadID:         createdLead.ID,
		DestinationOrganizationID: destinationTenantID,
		SourceLeadDeleted:         cleanupPlan.DeleteLead,
	}, nil
}

type quoteTransferCleanupPlan struct {
	DeleteLead bool
	ServiceID  *uuid.UUID
}

func planQuoteTransferSourceCleanup(_ uuid.UUID, sourceServiceID uuid.UUID, services []leadrepo.LeadService) quoteTransferCleanupPlan {
	if len(services) <= 1 {
		return quoteTransferCleanupPlan{DeleteLead: true}
	}
	serviceID := sourceServiceID
	return quoteTransferCleanupPlan{DeleteLead: false, ServiceID: &serviceID}
}

type quoteTransferAuditParams struct {
	quoteID               uuid.UUID
	leadID                uuid.UUID
	serviceID             *uuid.UUID
	tenantID              uuid.UUID
	relatedOrganizationID uuid.UUID
	actorID               uuid.UUID
	action                string
	message               string
}

func (s *Service) recordQuoteTransferAudit(ctx context.Context, params quoteTransferAuditParams) {
	metadata, _ := json.Marshal(map[string]any{
		"action":                params.action,
		"relatedOrganizationId": params.relatedOrganizationID,
	})
	_ = s.repo.CreateActivity(ctx, &repository.QuoteActivity{
		ID:             uuid.New(),
		QuoteID:        params.quoteID,
		OrganizationID: params.tenantID,
		EventType:      "transfer",
		Message:        params.message,
		Metadata:       metadata,
		CreatedAt:      time.Now(),
	})
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         params.leadID,
		ServiceID:      params.serviceID,
		OrganizationID: params.tenantID,
		ActorType:      "User",
		ActorName:      params.actorID.String(),
		EventType:      "quote_transfer",
		Title:          params.message,
		Summary:        nil,
		Metadata: map[string]any{
			"quoteId":               params.quoteID,
			"action":                params.action,
			"relatedOrganizationId": params.relatedOrganizationID,
		},
		Visibility:     "internal",
	})
}

type quoteTransferContext struct {
	sourceQuote    *repository.Quote
	sourceLead     leadrepo.Lead
	sourceServices []leadrepo.LeadService
	sourceService  *leadrepo.LeadService
	items          []repository.QuoteItem
	attachments    []repository.QuoteAttachment
	urls           []repository.QuoteURL
}

func (s *Service) loadQuoteTransferContext(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (quoteTransferContext, error) {
	sourceQuote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	items, attachments, urls, err := s.loadQuoteCloneData(ctx, id, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	sourceLead, sourceServices, err := s.leadRepo.GetByIDWithServices(ctx, sourceQuote.LeadID, tenantID)
	if err != nil {
		return quoteTransferContext{}, err
	}
	sourceService := resolveTransferredQuoteService(sourceQuote.LeadServiceID, sourceServices)
	if sourceService == nil {
		return quoteTransferContext{}, apperr.Validation("quote has no transferable service context")
	}

	return quoteTransferContext{
		sourceQuote:    sourceQuote,
		sourceLead:     sourceLead,
		sourceServices: sourceServices,
		sourceService:  sourceService,
		items:          items,
		attachments:    attachments,
		urls:           urls,
	}, nil
}

func (s *Service) createTransferredQuoteLead(ctx context.Context, transfer quoteTransferContext, destinationTenantID uuid.UUID) (leadstransport.LeadResponse, error) {
	createdLead, err := s.leadCreator.Create(ctx, leadstransport.CreateLeadRequest{
		FirstName:       firstNonEmptyQuoteString(transfer.sourceQuote.CustomerFirstName, &transfer.sourceLead.ConsumerFirstName),
		LastName:        firstNonEmptyQuoteString(transfer.sourceQuote.CustomerLastName, &transfer.sourceLead.ConsumerLastName),
		Phone:           firstNonEmptyQuoteString(transfer.sourceQuote.CustomerPhone, &transfer.sourceLead.ConsumerPhone),
		Email:           firstNonEmptyQuoteString(transfer.sourceQuote.CustomerEmail, transfer.sourceLead.ConsumerEmail),
		ConsumerRole:    leadstransport.ConsumerRole(transfer.sourceLead.ConsumerRole),
		Street:          firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressStreet, &transfer.sourceLead.AddressStreet),
		HouseNumber:     firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressHouseNumber, &transfer.sourceLead.AddressHouseNumber),
		ZipCode:         firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressZipCode, &transfer.sourceLead.AddressZipCode),
		City:            firstNonEmptyQuoteString(transfer.sourceQuote.CustomerAddressCity, &transfer.sourceLead.AddressCity),
		Latitude:        transfer.sourceLead.Latitude,
		Longitude:       transfer.sourceLead.Longitude,
		ServiceType:     leadstransport.ServiceType(transfer.sourceService.ServiceType),
		ConsumerNote:    ptrToString(transfer.sourceService.ConsumerNote),
		Source:          firstNonEmptyQuoteString(transfer.sourceService.Source, transfer.sourceLead.Source),
		WhatsAppOptedIn: quoteBoolPtr(transfer.sourceLead.WhatsAppOptedIn),
	}, destinationTenantID)
	if err != nil {
		return leadstransport.LeadResponse{}, err
	}

	if createdLead.CurrentService != nil {
		if _, err := s.leadRepo.UpdateServiceStatusAndPipelineStage(ctx, createdLead.CurrentService.ID, destinationTenantID, transfer.sourceService.Status, transfer.sourceService.PipelineStage); err != nil {
			return leadstransport.LeadResponse{}, err
		}
	}

	return createdLead, nil
}

func buildTransferredQuoteRequest(transfer quoteTransferContext, createdLead leadstransport.LeadResponse) transport.CreateQuoteRequest {
	request := transport.CreateQuoteRequest{
		LeadID:              createdLead.ID,
		PricingMode:         transfer.sourceQuote.PricingMode,
		DiscountType:        transfer.sourceQuote.DiscountType,
		DiscountValue:       transfer.sourceQuote.DiscountValue,
		ValidUntil:          transfer.sourceQuote.ValidUntil,
		Notes:               ptrToString(transfer.sourceQuote.Notes),
		FinancingDisclaimer: transfer.sourceQuote.FinancingDisclaimer,
		Items:               make([]transport.QuoteItemRequest, len(transfer.items)),
		Attachments:         cloneAttachmentRequests(transfer.attachments),
		URLs:                cloneURLRequests(transfer.urls),
	}
	if createdLead.CurrentService != nil {
		request.LeadServiceID = &createdLead.CurrentService.ID
	}
	for index, item := range transfer.items {
		request.Items[index] = transport.QuoteItemRequest{
			Title:            item.Title,
			Description:      item.Description,
			Quantity:         item.Quantity,
			UnitPriceCents:   item.UnitPriceCents,
			TaxRateBps:       item.TaxRateBps,
			IsOptional:       item.IsOptional,
			IsSelected:       item.IsSelected,
			CatalogProductID: item.CatalogProductID,
		}
	}
	return request
}

func resolveTransferredQuoteService(leadServiceID *uuid.UUID, services []leadrepo.LeadService) *leadrepo.LeadService {
	if leadServiceID != nil {
		for _, service := range services {
			if service.ID == *leadServiceID {
				serviceCopy := service
				return &serviceCopy
			}
		}
	}
	for _, service := range services {
		if service.PipelineStage != "Completed" && service.PipelineStage != "Lost" {
			serviceCopy := service
			return &serviceCopy
		}
	}
	if len(services) == 0 {
		return nil
	}
	serviceCopy := services[0]
	return &serviceCopy
}

func firstNonEmptyQuoteString(values ...*string) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func quoteBoolPtr(value bool) *bool {
	result := value
	return &result
}

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

func validateSendableQuoteStatus(status string) error {
	if status != string(transport.QuoteStatusDraft) && status != string(transport.QuoteStatusSent) {
		return apperr.BadRequest("only draft or sent quotes can be sent")
	}
	return nil
}

func (s *Service) ensureQuotePublicToken(ctx context.Context, quote *repository.Quote, tenantID uuid.UUID) (string, error) {
	token := strings.TrimSpace(ptrToString(quote.PublicToken))
	if token != "" {
		return token, nil
	}

	generatedToken, err := generatePublicToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(defaultPublicTokenTTL)
	if quote.ValidUntil != nil && quote.ValidUntil.After(time.Now()) {
		expiresAt = *quote.ValidUntil
	}
	if err := s.repo.SetPublicToken(ctx, quote.ID, tenantID, generatedToken, expiresAt); err != nil {
		return "", err
	}

	return generatedToken, nil
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

func (s *Service) GetPreviewLink(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*transport.QuotePreviewLinkResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if quote.Status == string(transport.QuoteStatusDraft) {
		return nil, apperr.BadRequest("preview link is not available for draft quotes")
	}
	now := time.Now()
	if quote.PreviewToken != nil && quote.PreviewTokenExpAt != nil && quote.PreviewTokenExpAt.After(now) {
		return &transport.QuotePreviewLinkResponse{Token: *quote.PreviewToken, ExpiresAt: quote.PreviewTokenExpAt}, nil
	}
	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(defaultPublicTokenTTL)
	if quote.ValidUntil != nil && quote.ValidUntil.After(now) {
		expiresAt = *quote.ValidUntil
	}
	if err := s.repo.SetPreviewToken(ctx, id, tenantID, token, expiresAt); err != nil {
		return nil, err
	}
	return &transport.QuotePreviewLinkResponse{Token: token, ExpiresAt: &expiresAt}, nil
}

func (s *Service) buildResponse(ctx context.Context, q *repository.Quote, items []repository.QuoteItem, annotations ...[]repository.QuoteAnnotation) (*transport.QuoteResponse, error) {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	annotationsByItem := make(map[uuid.UUID][]transport.AnnotationResponse)
	if len(annotations) > 0 {
		for _, a := range annotations[0] {
			annotationsByItem[a.QuoteItemID] = append(annotationsByItem[a.QuoteItemID], transport.AnnotationResponse{ID: a.ID, ItemID: a.QuoteItemID, AuthorType: a.AuthorType, AuthorID: a.AuthorID, Text: a.Text, IsResolved: a.IsResolved, CreatedAt: a.CreatedAt})
		}
	}

	respItems := make([]transport.QuoteItemResponse, len(items))
	for i, it := range items {
		qty := parseQuantityNumber(it.Quantity)
		unitPrice := float64(it.UnitPriceCents)
		taxRateBps := it.TaxRateBps
		netUnitPrice := unitPrice
		if pricingMode == "inclusive" && taxRateBps > 0 {
			netUnitPrice = unitPrice / (1.0 + float64(taxRateBps)/10000.0)
		}
		lineSubtotal := qty * netUnitPrice
		lineVat := lineSubtotal * (float64(taxRateBps) / 10000.0)
		respItems[i] = transport.QuoteItemResponse{ID: it.ID, Title: it.Title, Description: it.Description, Quantity: it.Quantity, UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: it.IsSelected, SortOrder: it.SortOrder, CatalogProductID: it.CatalogProductID, TotalBeforeTaxCents: roundCents(lineSubtotal), TotalTaxCents: roundCents(lineVat), LineTotalCents: roundCents(lineSubtotal + lineVat), Annotations: annotationsByItem[it.ID]}
		if respItems[i].Annotations == nil {
			respItems[i].Annotations = []transport.AnnotationResponse{}
		}
	}

	attachments, err := s.loadAttachmentResponses(ctx, q.ID, q.OrganizationID)
	if err != nil {
		return nil, err
	}
	urls, err := s.loadURLResponses(ctx, q.ID, q.OrganizationID)
	if err != nil {
		return nil, err
	}
	duplicatedFromQuoteNumber, previousVersionQuoteNumber, err := s.loadQuoteLineageNumbers(ctx, q)
	if err != nil {
		return nil, err
	}

	return &transport.QuoteResponse{ID: q.ID, QuoteNumber: q.QuoteNumber, DuplicatedFromQuoteID: q.DuplicatedFromQuoteID, DuplicatedFromQuoteNumber: duplicatedFromQuoteNumber, PreviousVersionQuoteID: q.PreviousVersionQuoteID, PreviousVersionQuoteNumber: previousVersionQuoteNumber, VersionRootQuoteID: q.VersionRootQuoteID, VersionNumber: q.VersionNumber, LeadID: q.LeadID, LeadServiceID: q.LeadServiceID, CreatedByID: q.CreatedByID, CreatedByFirstName: q.CreatedByFirstName, CreatedByLastName: q.CreatedByLastName, CreatedByEmail: q.CreatedByEmail, CustomerFirstName: q.CustomerFirstName, CustomerLastName: q.CustomerLastName, CustomerPhone: q.CustomerPhone, CustomerEmail: q.CustomerEmail, CustomerAddressStreet: q.CustomerAddressStreet, CustomerAddressHouseNumber: q.CustomerAddressHouseNumber, CustomerAddressZipCode: q.CustomerAddressZipCode, CustomerAddressCity: q.CustomerAddressCity, Status: transport.QuoteStatus(q.Status), PricingMode: q.PricingMode, DiscountType: q.DiscountType, DiscountValue: q.DiscountValue, SubtotalCents: q.SubtotalCents, DiscountAmountCents: q.DiscountAmountCents, TaxTotalCents: q.TaxTotalCents, TotalCents: q.TotalCents, ValidUntil: q.ValidUntil, Notes: q.Notes, Items: respItems, Attachments: attachments, URLs: urls, ViewedAt: q.ViewedAt, AcceptedAt: q.AcceptedAt, RejectedAt: q.RejectedAt, PDFFileKey: q.PDFFileKey, FinancingDisclaimer: q.FinancingDisclaimer, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt}, nil
}

func (s *Service) loadQuoteLineageNumbers(ctx context.Context, q *repository.Quote) (*string, *string, error) {
	var duplicatedFromQuoteNumber *string
	if q.DuplicatedFromQuoteID != nil {
		quoteNumber, err := s.repo.GetQuoteNumberByID(ctx, *q.DuplicatedFromQuoteID, q.OrganizationID)
		if err != nil {
			return nil, nil, err
		}
		duplicatedFromQuoteNumber = quoteNumber
	}

	var previousVersionQuoteNumber *string
	if q.PreviousVersionQuoteID != nil {
		quoteNumber, err := s.repo.GetQuoteNumberByID(ctx, *q.PreviousVersionQuoteID, q.OrganizationID)
		if err != nil {
			return nil, nil, err
		}
		previousVersionQuoteNumber = quoteNumber
	}

	return duplicatedFromQuoteNumber, previousVersionQuoteNumber, nil
}

func (s *Service) GetAttachmentByID(ctx context.Context, attachmentID, quoteID, tenantID uuid.UUID) (*repository.QuoteAttachment, error) {
	if _, err := s.repo.GetByID(ctx, quoteID, tenantID); err != nil {
		return nil, err
	}
	return s.repo.GetAttachmentByID(ctx, attachmentID, quoteID, tenantID)
}

func (s *Service) saveAttachments(ctx context.Context, quoteID, orgID uuid.UUID, reqs []transport.QuoteAttachmentRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	now := time.Now()
	models := make([]repository.QuoteAttachment, len(reqs))
	for i, r := range reqs {
		models[i] = repository.QuoteAttachment{ID: uuid.New(), QuoteID: quoteID, OrganizationID: orgID, Filename: r.Filename, FileKey: r.FileKey, Source: r.Source, CatalogProductID: r.CatalogProductID, Enabled: r.Enabled, SortOrder: r.SortOrder, CreatedAt: now}
	}
	return s.repo.ReplaceAttachments(ctx, quoteID, orgID, models)
}

func (s *Service) saveURLs(ctx context.Context, quoteID, orgID uuid.UUID, reqs []transport.QuoteURLRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	now := time.Now()
	models := make([]repository.QuoteURL, len(reqs))
	for i, r := range reqs {
		models[i] = repository.QuoteURL{ID: uuid.New(), QuoteID: quoteID, OrganizationID: orgID, Label: r.Label, Href: r.Href, Accepted: false, CatalogProductID: r.CatalogProductID, CreatedAt: now}
	}
	return s.repo.ReplaceURLs(ctx, quoteID, orgID, models)
}

func (s *Service) loadAttachmentResponses(ctx context.Context, quoteID, orgID uuid.UUID) ([]transport.QuoteAttachmentResponse, error) {
	attachments, err := s.repo.GetAttachmentsByQuoteID(ctx, quoteID, orgID)
	if err != nil {
		return nil, err
	}
	resp := make([]transport.QuoteAttachmentResponse, len(attachments))
	for i, a := range attachments {
		resp[i] = toAttachmentResponse(a)
	}
	return resp, nil
}

func (s *Service) loadURLResponses(ctx context.Context, quoteID, orgID uuid.UUID) ([]transport.QuoteURLResponse, error) {
	urls, err := s.repo.GetURLsByQuoteID(ctx, quoteID, orgID)
	if err != nil {
		return nil, err
	}
	resp := make([]transport.QuoteURLResponse, len(urls))
	for i, u := range urls {
		resp[i] = toURLResponse(u)
	}
	return resp, nil
}

func (s *Service) loadAttachmentResponsesNoOrg(ctx context.Context, quoteID uuid.UUID) ([]transport.QuoteAttachmentResponse, error) {
	attachments, err := s.repo.GetAttachmentsByQuoteIDNoOrg(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	resp := make([]transport.QuoteAttachmentResponse, len(attachments))
	for i, a := range attachments {
		resp[i] = toAttachmentResponse(a)
	}
	return resp, nil
}

func (s *Service) loadURLResponsesNoOrg(ctx context.Context, quoteID uuid.UUID) ([]transport.QuoteURLResponse, error) {
	urls, err := s.repo.GetURLsByQuoteIDNoOrg(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	resp := make([]transport.QuoteURLResponse, len(urls))
	for i, u := range urls {
		resp[i] = toURLResponse(u)
	}
	return resp, nil
}

func toAttachmentResponse(a repository.QuoteAttachment) transport.QuoteAttachmentResponse {
	return transport.QuoteAttachmentResponse{ID: a.ID, Filename: a.Filename, FileKey: a.FileKey, Source: a.Source, CatalogProductID: a.CatalogProductID, Enabled: a.Enabled, SortOrder: a.SortOrder, CreatedAt: a.CreatedAt}
}

func toURLResponse(u repository.QuoteURL) transport.QuoteURLResponse {
	return transport.QuoteURLResponse{ID: u.ID, Label: u.Label, Href: u.Href, Accepted: u.Accepted, CatalogProductID: u.CatalogProductID, CreatedAt: u.CreatedAt}
}

func (s *Service) emitTimelineEvent(ctx context.Context, params TimelineEventParams) {
	if s.timeline == nil {
		return
	}
	if err := s.timeline.CreateTimelineEvent(ctx, params); err != nil {
		return
	}
}

func toPtr(s string) *string { return &s }

func (s *Service) lookupContactNames(ctx context.Context, leadID, orgID uuid.UUID) (orgName, customerName string, logoFileKey *string) {
	if s.contacts == nil {
		return "", "", nil
	}
	data, err := s.contacts.GetQuoteContactData(ctx, leadID, orgID)
	if err != nil {
		return "", "", nil
	}
	return data.OrganizationName, data.ConsumerName, data.LogoFileKey
}

func clampPageSize(size int) int {
	if size < 1 || size > 100 {
		return 50
	}
	return size
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrToString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func parseDateRange(from string, to string, fromField string, toField string) (*time.Time, *time.Time, error) {
	const dateLayout = "2006-01-02"
	trimmedFrom := strings.TrimSpace(from)
	trimmedTo := strings.TrimSpace(to)
	var start *time.Time
	var end *time.Time
	if trimmedFrom != "" {
		parsed, err := time.Parse(dateLayout, trimmedFrom)
		if err != nil {
			return nil, nil, apperr.Validation(msgInvalidField + fromField)
		}
		start = &parsed
	}
	if trimmedTo != "" {
		parsed, err := time.Parse(dateLayout, trimmedTo)
		if err != nil {
			return nil, nil, apperr.Validation(msgInvalidField + toField)
		}
		endExclusive := parsed.AddDate(0, 0, 1)
		end = &endExclusive
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, apperr.Validation(fromField + " must be before " + toField)
	}
	return start, end, nil
}

func parseInt64Range(from string, to string, fromField string, toField string) (*int64, *int64, error) {
	trimmedFrom := strings.TrimSpace(from)
	trimmedTo := strings.TrimSpace(to)
	var start *int64
	var end *int64
	if trimmedFrom != "" {
		parsed, err := strconv.ParseInt(trimmedFrom, 10, 64)
		if err != nil {
			return nil, nil, apperr.Validation(msgInvalidField + fromField)
		}
		start = &parsed
	}
	if trimmedTo != "" {
		parsed, err := strconv.ParseInt(trimmedTo, 10, 64)
		if err != nil {
			return nil, nil, apperr.Validation(msgInvalidField + toField)
		}
		end = &parsed
	}
	if start != nil && end != nil && *start > *end {
		return nil, nil, apperr.Validation(fromField + " must be <= " + toField)
	}
	return start, end, nil
}

func (s *Service) ListActivities(ctx context.Context, quoteID, tenantID uuid.UUID) ([]transport.QuoteActivityResponse, error) {
	activities, err := s.repo.ListActivities(ctx, quoteID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list quote activities: %w", err)
	}
	out := make([]transport.QuoteActivityResponse, len(activities))
	for i, a := range activities {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			if err := json.Unmarshal(a.Metadata, &meta); err != nil {
				meta = nil
			}
		}
		out[i] = transport.QuoteActivityResponse{ID: a.ID, EventType: a.EventType, Message: a.Message, Metadata: meta, CreatedAt: a.CreatedAt}
	}
	return out, nil
}
