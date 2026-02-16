package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	draftMessageLanguageNL            = "nl"
	draftMessageAudienceCustomer      = "customer"
	draftMessageCategoryQuoteAccepted = "quote_accepted"
	draftMessageCategoryQuoteRejected = "quote_rejected"

	draftFallbackCustomerName = "klant"
	draftFallbackOrgName      = "ons team"
	draftFallbackRejectReason = "Geen reden opgegeven"

	draftAcceptedSubjectTemplate  = "Bevestiging ontvangen - offerte %s"
	draftAcceptedBodyTemplate     = "Hallo %s,\n\nBedankt voor het accepteren van offerte %s. Wij verwerken uw akkoord en nemen snel contact met u op voor de volgende stappen.\n\nOndertekend door: %s\nTotaal: EUR %.2f\n\nMet vriendelijke groet,\n%s"
	draftAcceptedWhatsAppTemplate = "Hallo %s, bedankt voor het accepteren van offerte %s. Wij nemen snel contact met u op voor de volgende stappen.\n\nMet vriendelijke groet, %s"

	draftRejectedSubjectTemplate  = "We hebben uw beslissing ontvangen - offerte %s"
	draftRejectedBodyTemplate     = "Hallo %s,\n\nWij hebben uw beslissing over offerte %s ontvangen. Reden: %s.\n\nAls u vragen heeft of wilt overleggen, helpen wij graag.\n\nMet vriendelijke groet,\n%s"
	draftRejectedWhatsAppTemplate = "Hallo %s, wij hebben uw beslissing over offerte %s ontvangen. Reden: %s. Als u vragen heeft, helpen wij graag.\n\nMet vriendelijke groet, %s"
)

func (s *Service) GetPublic(ctx context.Context, token string) (*transport.PublicQuoteResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	readOnly := isReadOnlyToken(tokenKind)
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}

	if !readOnly && quote.ViewedAt == nil {
		if err := s.repo.SetViewedAt(ctx, quote.ID); err != nil {
			return nil, err
		}
		now := time.Now()
		quote.ViewedAt = &now
		if s.eventBus != nil {
			s.eventBus.Publish(ctx, events.QuoteViewed{BaseEvent: events.NewBaseEvent(), QuoteID: quote.ID, OrganizationID: quote.OrganizationID, LeadID: quote.LeadID})
		}
	}

	items, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}
	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	return s.buildPublicResponse(ctx, quote, items, orgName, customerName, readOnly)
}

func (s *Service) ToggleLineItem(ctx context.Context, token string, itemID uuid.UUID, req transport.ToggleItemRequest) (*transport.ToggleItemResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if isReadOnlyToken(tokenKind) {
		return nil, apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}
	if quote.Status == string(transport.QuoteStatusAccepted) || quote.Status == string(transport.QuoteStatusRejected) {
		return nil, apperr.BadRequest(msgAlreadyFinal)
	}

	item, err := s.repo.GetItemByID(ctx, itemID, quote.ID)
	if err != nil {
		return nil, err
	}
	if !item.IsOptional {
		return nil, apperr.BadRequest("only optional items can be toggled")
	}
	if err := s.repo.UpdateItemSelection(ctx, itemID, quote.ID, req.IsSelected); err != nil {
		return nil, err
	}

	allItems, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}
	itemReqs := make([]transport.QuoteItemRequest, len(allItems))
	for i, it := range allItems {
		selected := it.IsSelected
		if it.ID == itemID {
			selected = req.IsSelected
		}
		itemReqs[i] = transport.QuoteItemRequest{Description: it.Description, Quantity: it.Quantity, UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: selected}
	}
	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: itemReqs, PricingMode: quote.PricingMode, DiscountType: quote.DiscountType, DiscountValue: quote.DiscountValue})
	if err := s.repo.UpdateQuoteTotals(ctx, quote.ID, calc.SubtotalCents, calc.DiscountAmountCents, calc.VatTotalCents, calc.TotalCents); err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteUpdatedByCustomer{BaseEvent: events.NewBaseEvent(), QuoteID: quote.ID, OrganizationID: quote.OrganizationID, ItemID: itemID, ItemDescription: item.Description, IsSelected: req.IsSelected, NewTotalCents: calc.TotalCents})
	}
	return &transport.ToggleItemResponse{SubtotalCents: calc.SubtotalCents, DiscountAmountCents: calc.DiscountAmountCents, TaxTotalCents: calc.VatTotalCents, TotalCents: calc.TotalCents, VatBreakdown: calc.VatBreakdown}, nil
}

func (s *Service) AnnotateItem(ctx context.Context, token string, itemID uuid.UUID, authorType, authorID, text string) (*transport.AnnotationResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if isReadOnlyToken(tokenKind) {
		return nil, apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}
	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return nil, err
	}
	var authorUUID *uuid.UUID
	if authorID != "" {
		if parsed, parseErr := uuid.Parse(authorID); parseErr == nil {
			authorUUID = &parsed
		}
	}
	annotation := repository.QuoteAnnotation{ID: uuid.New(), QuoteItemID: itemID, OrganizationID: quote.OrganizationID, AuthorType: authorType, AuthorID: authorUUID, Text: text, IsResolved: false, CreatedAt: time.Now()}
	if err := s.repo.CreateAnnotation(ctx, &annotation); err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteAnnotated{BaseEvent: events.NewBaseEvent(), QuoteID: quote.ID, OrganizationID: quote.OrganizationID, ItemID: itemID, AuthorType: authorType, AuthorID: authorID, Text: text})
	}
	return &transport.AnnotationResponse{ID: annotation.ID, ItemID: annotation.QuoteItemID, AuthorType: annotation.AuthorType, AuthorID: annotation.AuthorID, Text: annotation.Text, IsResolved: annotation.IsResolved, CreatedAt: annotation.CreatedAt}, nil
}

func (s *Service) UpdateAnnotation(ctx context.Context, token string, itemID, annotationID uuid.UUID, authorType, text string) (*transport.AnnotationResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if isReadOnlyToken(tokenKind) {
		return nil, apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}
	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return nil, err
	}
	annotation, err := s.repo.UpdateAnnotationText(ctx, annotationID, itemID, authorType, text)
	if err != nil {
		return nil, err
	}
	return &transport.AnnotationResponse{ID: annotation.ID, ItemID: annotation.QuoteItemID, AuthorType: annotation.AuthorType, AuthorID: annotation.AuthorID, Text: annotation.Text, IsResolved: annotation.IsResolved, CreatedAt: annotation.CreatedAt}, nil
}

func (s *Service) DeleteAnnotation(ctx context.Context, token string, itemID, annotationID uuid.UUID, authorType string) error {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return err
	}
	if isReadOnlyToken(tokenKind) {
		return apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return apperr.Gone(msgLinkExpired)
	}
	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return err
	}
	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, quote.ID)
	if err != nil {
		return err
	}
	for _, ann := range annotations {
		if ann.QuoteItemID == itemID && ann.AuthorType == "agent" {
			return apperr.Forbidden("annotation cannot be deleted after agent response")
		}
	}
	return s.repo.DeleteAnnotation(ctx, annotationID, itemID, authorType)
}

func (s *Service) AgentAnnotateItem(ctx context.Context, quoteID, itemID, tenantID, agentID uuid.UUID, text string) (*transport.AnnotationResponse, error) {
	quote, err := s.repo.GetByID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return nil, err
	}
	annotation := repository.QuoteAnnotation{ID: uuid.New(), QuoteItemID: itemID, OrganizationID: tenantID, AuthorType: "agent", AuthorID: &agentID, Text: text, IsResolved: false, CreatedAt: time.Now()}
	if err := s.repo.CreateAnnotation(ctx, &annotation); err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteAnnotated{BaseEvent: events.NewBaseEvent(), QuoteID: quoteID, OrganizationID: tenantID, ItemID: itemID, AuthorType: "agent", AuthorID: agentID.String(), Text: text})
	}
	return &transport.AnnotationResponse{ID: annotation.ID, ItemID: annotation.QuoteItemID, AuthorType: annotation.AuthorType, AuthorID: annotation.AuthorID, Text: annotation.Text, IsResolved: annotation.IsResolved, CreatedAt: annotation.CreatedAt}, nil
}

func (s *Service) Accept(ctx context.Context, token string, req transport.AcceptQuoteRequest, clientIP string) (*transport.PublicQuoteResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if isReadOnlyToken(tokenKind) {
		return nil, apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}
	if quote.Status == string(transport.QuoteStatusAccepted) {
		return nil, apperr.BadRequest("this quote has already been accepted")
	}
	if quote.Status == string(transport.QuoteStatusRejected) {
		return nil, apperr.BadRequest("this quote has been rejected")
	}
	if err := s.repo.AcceptQuote(ctx, quote.ID, req.SignatureName, req.SignatureData, clientIP); err != nil {
		return nil, err
	}
	quote, _, err = s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		evt := events.QuoteAccepted{BaseEvent: events.NewBaseEvent(), QuoteID: quote.ID, OrganizationID: quote.OrganizationID, LeadID: quote.LeadID, LeadServiceID: quote.LeadServiceID, SignatureName: req.SignatureName, TotalCents: quote.TotalCents, QuoteNumber: quote.QuoteNumber}
		if s.contacts != nil {
			if contactData, lookupErr := s.contacts.GetQuoteContactData(ctx, quote.LeadID, quote.OrganizationID); lookupErr == nil {
				evt.ConsumerEmail = contactData.ConsumerEmail
				evt.ConsumerName = contactData.ConsumerName
				evt.ConsumerPhone = contactData.ConsumerPhone
				evt.OrganizationName = contactData.OrganizationName
				evt.AgentEmail = contactData.AgentEmail
				evt.AgentName = contactData.AgentName
			}
		}
		s.eventBus.Publish(ctx, evt)
	}

	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	drafts := buildQuoteAcceptedDrafts(quote.QuoteNumber, orgName, customerName, req.SignatureName, quote.TotalCents)
	s.emitTimelineEvent(ctx, TimelineEventParams{LeadID: quote.LeadID, ServiceID: quote.LeadServiceID, OrganizationID: quote.OrganizationID, ActorType: "Lead", ActorName: req.SignatureName, EventType: "quote_accepted", Title: fmt.Sprintf("Quote %s accepted", quote.QuoteNumber), Summary: toPtr(fmt.Sprintf("Signed by %s — "+msgTotalFormat, req.SignatureName, float64(quote.TotalCents)/100)), Metadata: map[string]any{"quoteId": quote.ID, "status": "Accepted", "signatureName": req.SignatureName, "drafts": drafts}})
	return s.buildPublicResponse(ctx, quote, items, orgName, customerName, false)
}

func (s *Service) Reject(ctx context.Context, token string, req transport.RejectQuoteRequest) (*transport.PublicQuoteResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if isReadOnlyToken(tokenKind) {
		return nil, apperr.Forbidden(msgReadOnly)
	}
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}
	if quote.Status == string(transport.QuoteStatusAccepted) || quote.Status == string(transport.QuoteStatusRejected) {
		return nil, apperr.BadRequest(msgAlreadyFinal)
	}
	reasonPtr := &req.Reason
	if req.Reason == "" {
		reasonPtr = nil
	}
	if err := s.repo.RejectQuote(ctx, quote.ID, reasonPtr); err != nil {
		return nil, err
	}
	quote, _, err = s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}
	s.publishQuoteRejectedEvent(ctx, quote, req.Reason)
	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	drafts := buildQuoteRejectedDrafts(quote.QuoteNumber, orgName, customerName, req.Reason)
	s.emitTimelineEvent(ctx, TimelineEventParams{LeadID: quote.LeadID, ServiceID: quote.LeadServiceID, OrganizationID: quote.OrganizationID, ActorType: "Lead", ActorName: "Customer", EventType: "quote_rejected", Title: fmt.Sprintf("Quote %s rejected", quote.QuoteNumber), Summary: nilIfEmpty(req.Reason), Metadata: map[string]any{"quoteId": quote.ID, "status": "Rejected", "reason": req.Reason, "drafts": drafts}})
	return s.buildPublicResponse(ctx, quote, items, orgName, customerName, false)
}

func (s *Service) publishQuoteRejectedEvent(ctx context.Context, quote *repository.Quote, reason string) {
	if s.eventBus == nil {
		return
	}
	evt := events.QuoteRejected{BaseEvent: events.NewBaseEvent(), QuoteID: quote.ID, OrganizationID: quote.OrganizationID, LeadID: quote.LeadID, LeadServiceID: quote.LeadServiceID, Reason: reason}
	if s.contacts != nil {
		if contactData, lookupErr := s.contacts.GetQuoteContactData(ctx, quote.LeadID, quote.OrganizationID); lookupErr == nil {
			evt.ConsumerEmail = contactData.ConsumerEmail
			evt.ConsumerName = contactData.ConsumerName
			evt.ConsumerPhone = contactData.ConsumerPhone
			evt.OrganizationName = contactData.OrganizationName
		}
	}
	s.eventBus.Publish(ctx, evt)
}

func (s *Service) buildPublicResponse(ctx context.Context, q *repository.Quote, items []repository.QuoteItem, organizationName, customerName string, readOnly bool) (*transport.PublicQuoteResponse, error) {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	annotations, err := s.repo.ListAnnotationsByQuoteID(ctx, q.ID)
	if err != nil {
		annotations = nil
	}
	annotationsByItem := make(map[uuid.UUID][]transport.AnnotationResponse)
	for _, ann := range annotations {
		annotationsByItem[ann.QuoteItemID] = append(annotationsByItem[ann.QuoteItemID], transport.AnnotationResponse{ID: ann.ID, ItemID: ann.QuoteItemID, AuthorType: ann.AuthorType, AuthorID: ann.AuthorID, Text: ann.Text, IsResolved: ann.IsResolved, CreatedAt: ann.CreatedAt})
	}

	respItems := make([]transport.PublicQuoteItemResponse, len(items))
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
		respItems[i] = transport.PublicQuoteItemResponse{ID: it.ID, Description: it.Description, Quantity: it.Quantity, UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: it.IsSelected, SortOrder: it.SortOrder, TotalBeforeTaxCents: roundCents(lineSubtotal), TotalTaxCents: roundCents(lineVat), LineTotalCents: roundCents(lineSubtotal + lineVat), Annotations: annotationsByItem[it.ID]}
		if respItems[i].Annotations == nil {
			respItems[i].Annotations = []transport.AnnotationResponse{}
		}
	}

	itemReqs := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		itemReqs[i] = transport.QuoteItemRequest{Description: it.Description, Quantity: it.Quantity, UnitPriceCents: it.UnitPriceCents, TaxRateBps: it.TaxRateBps, IsOptional: it.IsOptional, IsSelected: it.IsSelected}
	}
	calc := CalculateQuote(transport.QuoteCalculationRequest{Items: itemReqs, PricingMode: q.PricingMode, DiscountType: q.DiscountType, DiscountValue: q.DiscountValue})

	attachments, err := s.loadAttachmentResponsesNoOrg(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	urls, err := s.loadURLResponsesNoOrg(ctx, q.ID)
	if err != nil {
		return nil, err
	}

	return &transport.PublicQuoteResponse{ID: q.ID, QuoteNumber: q.QuoteNumber, Status: transport.QuoteStatus(q.Status), PricingMode: q.PricingMode, OrganizationName: organizationName, CustomerName: customerName, DiscountType: q.DiscountType, DiscountValue: q.DiscountValue, SubtotalCents: calc.SubtotalCents, DiscountAmountCents: calc.DiscountAmountCents, TaxTotalCents: calc.VatTotalCents, TotalCents: calc.TotalCents, VatBreakdown: calc.VatBreakdown, ValidUntil: q.ValidUntil, Notes: q.Notes, Items: respItems, Attachments: attachments, URLs: urls, AcceptedAt: q.AcceptedAt, RejectedAt: q.RejectedAt, FinancingDisclaimer: q.FinancingDisclaimer, IsReadOnly: readOnly}, nil
}

func buildQuoteAcceptedDrafts(quoteNumber, orgName, customerName, signatureName string, totalCents int64) map[string]any {
	if strings.TrimSpace(customerName) == "" {
		customerName = draftFallbackCustomerName
	}
	if strings.TrimSpace(orgName) == "" {
		orgName = draftFallbackOrgName
	}
	subject := fmt.Sprintf(draftAcceptedSubjectTemplate, quoteNumber)
	body := fmt.Sprintf(draftAcceptedBodyTemplate, customerName, quoteNumber, signatureName, float64(totalCents)/100, orgName)
	whatsApp := fmt.Sprintf(draftAcceptedWhatsAppTemplate, customerName, quoteNumber, orgName)
	return map[string]any{"emailSubject": subject, "emailBody": body, "whatsappMessage": whatsApp, "messageLanguage": draftMessageLanguageNL, "messageAudience": draftMessageAudienceCustomer, "messageCategory": draftMessageCategoryQuoteAccepted}
}

func buildQuoteRejectedDrafts(quoteNumber, orgName, customerName, reason string) map[string]any {
	if strings.TrimSpace(customerName) == "" {
		customerName = draftFallbackCustomerName
	}
	if strings.TrimSpace(orgName) == "" {
		orgName = draftFallbackOrgName
	}
	cleanReason := strings.TrimSpace(reason)
	if cleanReason == "" {
		cleanReason = draftFallbackRejectReason
	}
	subject := fmt.Sprintf(draftRejectedSubjectTemplate, quoteNumber)
	body := fmt.Sprintf(draftRejectedBodyTemplate, customerName, quoteNumber, cleanReason, orgName)
	whatsApp := fmt.Sprintf(draftRejectedWhatsAppTemplate, customerName, quoteNumber, cleanReason, orgName)
	return map[string]any{"emailSubject": subject, "emailBody": body, "whatsappMessage": whatsApp, "messageLanguage": draftMessageLanguageNL, "messageAudience": draftMessageAudienceCustomer, "messageCategory": draftMessageCategoryQuoteRejected}
}
