package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/events"
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
	defaultTaxRateBps     = 2100
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

	validUntil := req.ValidUntil.ToTimePtr()
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
		PagePerItem:         req.PagePerItem,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := applyQuoteSubsidySnapshot(&quote, req.ISDESubsidy); err != nil {
		return nil, err
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
		quote.ValidUntil = req.ValidUntil.ToTimePtr()
	}
	if req.Notes != nil {
		quote.Notes = req.Notes
	}
	if req.FinancingDisclaimer != nil {
		quote.FinancingDisclaimer = *req.FinancingDisclaimer
	}
	if req.PagePerItem != nil {
		quote.PagePerItem = *req.PagePerItem
	}
}

func marshalQuoteSubsidySnapshot(snapshot *transport.QuoteISDESubsidy) ([]byte, error) {
	if snapshot == nil {
		return nil, nil
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal quote subsidy snapshot: %w", err)
	}
	return data, nil
}

func unmarshalQuoteSubsidySnapshot(data []byte) (*transport.QuoteISDESubsidy, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var snapshot transport.QuoteISDESubsidy
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal quote subsidy snapshot: %w", err)
	}
	return &snapshot, nil
}

func quoteSubsidyEventPayload(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload
}

func applyQuoteSubsidySnapshot(quote *repository.Quote, snapshot *transport.QuoteISDESubsidy) error {
	if snapshot == nil {
		return nil
	}
	data, err := marshalQuoteSubsidySnapshot(snapshot)
	if err != nil {
		return err
	}
	quote.SubsidyData = data
	return nil
}

func (s *Service) resolveQuoteUpdateItems(ctx context.Context, quoteID, tenantID uuid.UUID, req transport.UpdateQuoteRequest) ([]repository.QuoteItem, error) {
	if req.Items != nil {
		return buildItemsFromRequest(quoteID, tenantID, *req.Items), nil
	}
	return s.repo.GetItemsByQuoteID(ctx, quoteID, tenantID)
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

	if err := s.validateQuoteUpdate(quote, req); err != nil {
		return nil, err
	}

	pdfShouldInvalidate := quoteUpdateAffectsRenderedPDF(req)
	applyQuoteUpdates(quote, req)
	if err := applyQuoteSubsidySnapshot(quote, req.ISDESubsidy); err != nil {
		return nil, err
	}

	if err := s.syncTokenExpirationsWithValidUntil(ctx, quote, tenantID); err != nil {
		return nil, err
	}

	items, err := s.resolveQuoteUpdateItems(ctx, quote.ID, tenantID, req)
	if err != nil {
		return nil, err
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

	if err := s.persistQuoteUpdate(ctx, quote, items, tenantID, actorID, req, pdfShouldInvalidate); err != nil {
		return nil, err
	}

	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, quote.ID)
	if err != nil {
		annotations = nil
	}

	return s.buildResponse(ctx, quote, items, annotations)
}

func (s *Service) validateQuoteUpdate(quote *repository.Quote, req transport.UpdateQuoteRequest) error {
	if req.ValidUntil != nil && quote.Status != string(transport.QuoteStatusDraft) &&
		quote.Status != string(transport.QuoteStatusSent) &&
		quote.Status != string(transport.QuoteStatusAccepted) {
		return apperr.Validation(fmt.Sprintf("cannot extend quote with status '%s'; only Draft, Sent, or Accepted quotes can be extended", quote.Status))
	}
	return nil
}

func (s *Service) syncTokenExpirationsWithValidUntil(ctx context.Context, quote *repository.Quote, tenantID uuid.UUID) error {
	if quote.ValidUntil == nil {
		return nil
	}
	newValidUntil := *quote.ValidUntil
	if quote.PublicToken != nil && (quote.PublicTokenExpAt == nil || quote.PublicTokenExpAt.Before(newValidUntil)) {
		if err := s.repo.SetPublicToken(ctx, quote.ID, tenantID, *quote.PublicToken, newValidUntil); err != nil {
			return err
		}
		quote.PublicTokenExpAt = &newValidUntil
	}
	if quote.PreviewToken != nil && (quote.PreviewTokenExpAt == nil || quote.PreviewTokenExpAt.Before(newValidUntil)) {
		if err := s.repo.SetPreviewToken(ctx, quote.ID, tenantID, *quote.PreviewToken, newValidUntil); err != nil {
			return err
		}
		quote.PreviewTokenExpAt = &newValidUntil
	}
	return nil
}

func (s *Service) persistQuoteUpdate(ctx context.Context, quote *repository.Quote, items []repository.QuoteItem, tenantID uuid.UUID, actorID uuid.UUID, req transport.UpdateQuoteRequest, pdfShouldInvalidate bool) error {
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
		return err
	}
	if req.Attachments != nil {
		if err := s.saveAttachments(ctx, quote.ID, tenantID, *req.Attachments); err != nil {
			return fmt.Errorf(errSaveAttachmentsFmt, err)
		}
	}
	if req.URLs != nil {
		if err := s.saveURLs(ctx, quote.ID, tenantID, *req.URLs); err != nil {
			return fmt.Errorf(errSaveURLsFmt, err)
		}
	}
	return s.invalidateRenderedPDF(ctx, quote, pdfShouldInvalidate)
}

func quoteUpdateAffectsRenderedPDF(req transport.UpdateQuoteRequest) bool {
	return req.PricingMode != nil ||
		req.DiscountType != nil ||
		req.DiscountValue != nil ||
		req.ValidUntil != nil ||
		req.Notes != nil ||
		req.Items != nil ||
		req.Attachments != nil ||
		req.URLs != nil ||
		req.ISDESubsidy != nil ||
		req.FinancingDisclaimer != nil ||
		req.PagePerItem != nil
}

func (s *Service) invalidateRenderedPDF(ctx context.Context, quote *repository.Quote, shouldInvalidate bool) error {
	if !shouldInvalidate {
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

	if err := s.repo.UpdateWithItems(ctx, quote, updatedItems, true, &repository.QuotePricingSnapshot{
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
	}); err != nil {
		return err
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: organizationID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      "quote_extra_work_added",
		Title:          fmt.Sprintf("Extra work added to quote %s", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(quote.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId":        quote.ID,
			"extraWorkCents": amountCents,
			"status":         quote.Status,
		},
	})

	return nil
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



func (s *Service) List(ctx context.Context, tenantID uuid.UUID, req transport.ListQuotesRequest) (*transport.QuoteListResponse, error) {
	params, err := s.buildListParams(tenantID, req)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	quoteIDs := make([]uuid.UUID, 0, len(result.Items))
	for _, q := range result.Items {
		quoteIDs = append(quoteIDs, q.ID)
	}

	itemsByQuoteID, attachmentsByQuoteID, urlsByQuoteID, err := s.loadQuoteListRelatedData(ctx, tenantID, quoteIDs)
	if err != nil {
		return nil, err
	}

	items := make([]transport.QuoteResponse, len(result.Items))
	for i, q := range result.Items {
		attachments := toAttachmentResponses(attachmentsByQuoteID[q.ID])
		urls := toURLResponses(urlsByQuoteID[q.ID])
		mapped, mapErr := s.assembleQuoteResponse(&q, itemsByQuoteID[q.ID], nil, attachments, urls, nil, nil)
		if mapErr != nil {
			return nil, mapErr
		}
		items[i] = *mapped
	}

	return &transport.QuoteListResponse{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}, nil
}

func (s *Service) buildListParams(tenantID uuid.UUID, req transport.ListQuotesRequest) (repository.ListParams, error) {
	params := repository.ListParams{
		OrganizationID: tenantID,
		Status:         nilIfEmpty(req.Status),
		Search:         req.Search,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
		Page:           max(req.Page, 1),
		PageSize:       clampPageSize(req.PageSize),
	}

	createdFrom, createdTo, err := parseDateRange(req.CreatedAtFrom, req.CreatedAtTo, "createdAtFrom", "createdAtTo")
	if err != nil {
		return repository.ListParams{}, err
	}
	validFrom, validTo, err := parseDateRange(req.ValidUntilFrom, req.ValidUntilTo, "validUntilFrom", "validUntilTo")
	if err != nil {
		return repository.ListParams{}, err
	}
	totalFrom, totalTo, err := parseInt64Range(req.TotalFrom, req.TotalTo, "totalFrom", "totalTo")
	if err != nil {
		return repository.ListParams{}, err
	}

	params.CreatedAtFrom = createdFrom
	params.CreatedAtTo = createdTo
	params.ValidUntilFrom = validFrom
	params.ValidUntilTo = validTo
	params.TotalFrom = totalFrom
	params.TotalTo = totalTo

	if req.LeadID != "" {
		parsed, err := uuid.Parse(req.LeadID)
		if err != nil {
			return repository.ListParams{}, apperr.BadRequest("invalid leadId format")
		}
		params.LeadID = &parsed
	}

	return params, nil
}

func (s *Service) loadQuoteListRelatedData(ctx context.Context, tenantID uuid.UUID, quoteIDs []uuid.UUID) (map[uuid.UUID][]repository.QuoteItem, map[uuid.UUID][]repository.QuoteAttachment, map[uuid.UUID][]repository.QuoteURL, error) {
	itemsByQuoteID, err := s.repo.GetItemsByQuoteIDs(ctx, tenantID, quoteIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	attachmentsByQuoteID, err := s.repo.GetAttachmentsByQuoteIDs(ctx, tenantID, quoteIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	urlsByQuoteID, err := s.repo.GetURLsByQuoteIDs(ctx, tenantID, quoteIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	return itemsByQuoteID, attachmentsByQuoteID, urlsByQuoteID, nil
}

func toAttachmentResponses(attachments []repository.QuoteAttachment) []transport.QuoteAttachmentResponse {
	result := make([]transport.QuoteAttachmentResponse, len(attachments))
	for i, a := range attachments {
		result[i] = toAttachmentResponse(a)
	}
	return result
}

func toURLResponses(urls []repository.QuoteURL) []transport.QuoteURLResponse {
	result := make([]transport.QuoteURLResponse, len(urls))
	for i, u := range urls {
		result[i] = toURLResponse(u)
	}
	return result
}

func (s *Service) ListPendingApprovals(ctx context.Context, tenantID uuid.UUID, req transport.ListPendingApprovalsRequest) (*transport.PendingApprovalsResponse, error) {
	result, err := s.repo.ListPendingApprovals(ctx, tenantID, max(req.Page, 1), clampPageSize(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]transport.PendingApprovalItem, len(result.Items))
	for i, row := range result.Items {
		consumerName := strings.TrimSpace(ptrToString(row.ConsumerFirstName) + " " + ptrToString(row.ConsumerLastName))
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


// buildResponse loads all related data for a single quote and assembles the transport response.
func (s *Service) buildResponse(ctx context.Context, q *repository.Quote, items []repository.QuoteItem, annotations ...[]repository.QuoteAnnotation) (*transport.QuoteResponse, error) {
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

	var annotationSlice []repository.QuoteAnnotation
	if len(annotations) > 0 {
		annotationSlice = annotations[0]
	}

	return s.assembleQuoteResponse(q, items, annotationSlice, attachments, urls, duplicatedFromQuoteNumber, previousVersionQuoteNumber)
}

// assembleQuoteResponse maps fully loaded domain data to a transport response without performing any database calls.
func (s *Service) assembleQuoteResponse(q *repository.Quote, items []repository.QuoteItem, annotations []repository.QuoteAnnotation, attachments []transport.QuoteAttachmentResponse, urls []transport.QuoteURLResponse, duplicatedFromQuoteNumber, previousVersionQuoteNumber *string) (*transport.QuoteResponse, error) {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	annotationsByItem := make(map[uuid.UUID][]transport.AnnotationResponse)
	for _, a := range annotations {
		annotationsByItem[a.QuoteItemID] = append(annotationsByItem[a.QuoteItemID], transport.AnnotationResponse{ID: a.ID, ItemID: a.QuoteItemID, AuthorType: a.AuthorType, AuthorID: a.AuthorID, Text: a.Text, IsResolved: a.IsResolved, CreatedAt: a.CreatedAt})
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

	isdeSubsidy, err := unmarshalQuoteSubsidySnapshot(q.SubsidyData)
	if err != nil {
		return nil, err
	}

	return &transport.QuoteResponse{
		ID:                        q.ID,
		QuoteNumber:               q.QuoteNumber,
		DuplicatedFromQuoteID:     q.DuplicatedFromQuoteID,
		DuplicatedFromQuoteNumber: duplicatedFromQuoteNumber,
		PreviousVersionQuoteID:    q.PreviousVersionQuoteID,
		PreviousVersionQuoteNumber: previousVersionQuoteNumber,
		VersionRootQuoteID:        q.VersionRootQuoteID,
		VersionNumber:             q.VersionNumber,
		LeadID:                    q.LeadID,
		LeadServiceID:             q.LeadServiceID,
		CreatedByID:               q.CreatedByID,
		CreatedByFirstName:        q.CreatedByFirstName,
		CreatedByLastName:         q.CreatedByLastName,
		CreatedByEmail:            q.CreatedByEmail,
		CustomerFirstName:         q.CustomerFirstName,
		CustomerLastName:          q.CustomerLastName,
		CustomerPhone:             q.CustomerPhone,
		CustomerEmail:             q.CustomerEmail,
		CustomerAddressStreet:     q.CustomerAddressStreet,
		CustomerAddressHouseNumber: q.CustomerAddressHouseNumber,
		CustomerAddressZipCode:    q.CustomerAddressZipCode,
		CustomerAddressCity:       q.CustomerAddressCity,
		Status:                    transport.QuoteStatus(q.Status),
		PricingMode:               q.PricingMode,
		DiscountType:              q.DiscountType,
		DiscountValue:             q.DiscountValue,
		SubtotalCents:             q.SubtotalCents,
		DiscountAmountCents:       q.DiscountAmountCents,
		TaxTotalCents:             q.TaxTotalCents,
		TotalCents:                q.TotalCents,
		ValidUntil:                q.ValidUntil,
		Notes:                     q.Notes,
		ISDESubsidy:               isdeSubsidy,
		Items:                     respItems,
		Attachments:               attachments,
		URLs:                      urls,
		ViewedAt:                  q.ViewedAt,
		AcceptedAt:                q.AcceptedAt,
		RejectedAt:                q.RejectedAt,
		PDFFileKey:                q.PDFFileKey,
		FinancingDisclaimer:       q.FinancingDisclaimer,
		PagePerItem:               q.PagePerItem,
		CreatedAt:                 q.CreatedAt,
		UpdatedAt:                 q.UpdatedAt,
	}, nil
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
	trimmedFrom := strings.TrimSpace(from)
	trimmedTo := strings.TrimSpace(to)
	var start *time.Time
	var end *time.Time
	if trimmedFrom != "" {
		parsed, err := time.Parse("2006-01-02", trimmedFrom)
		if err != nil {
			return nil, nil, apperr.Validation(msgInvalidField + fromField)
		}
		start = &parsed
	}
	if trimmedTo != "" {
		parsed, err := time.Parse("2006-01-02", trimmedTo)
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
