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
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

// Shared message constants to avoid duplicated string literals.
const (
	msgTotalFormat  = "Totaal: €%.2f"
	msgLinkExpired  = "this quote link has expired"
	msgAlreadyFinal = "this quote has already been finalized"
	msgReadOnly     = "this preview link is read-only"
	msgInvalidField = "invalid "
)

// TimelineWriter is the narrow interface a quotes service needs to create lead timeline events.
// Implemented by an adapter in internal/adapters that wraps the leads repository.
type TimelineWriter interface {
	CreateTimelineEvent(ctx context.Context, params TimelineEventParams) error
}

// TimelineEventParams captures timeline event data without importing the leads domain.
type TimelineEventParams struct {
	LeadID         uuid.UUID
	ServiceID      *uuid.UUID
	OrganizationID uuid.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        *string
	Metadata       map[string]any
}

// QuoteContactData holds the consumer/organization/agent info needed for quote emails.
type QuoteContactData struct {
	ConsumerEmail    string
	ConsumerName     string
	OrganizationName string
	AgentEmail       string
	AgentName        string
}

// QuoteContactReader is a narrow interface the quotes service uses to look up lead and
// organization contact details for sending quote proposal emails.
// Implemented by an adapter in internal/adapters that wraps the leads + identity repositories.
type QuoteContactReader interface {
	GetQuoteContactData(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (QuoteContactData, error)
}

// Service provides business logic for quotes
type Service struct {
	repo     *repository.Repository
	timeline TimelineWriter     // optional — nil means no timeline integration
	eventBus events.Bus         // optional — nil means no event publishing
	contacts QuoteContactReader // optional — nil means no email enrichment
}

// New creates a new quotes service
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// SetTimelineWriter injects the timeline writer (set after construction to break circular deps).
func (s *Service) SetTimelineWriter(tw TimelineWriter) {
	s.timeline = tw
}

// SetEventBus injects the event bus (set after construction to break circular deps).
func (s *Service) SetEventBus(bus events.Bus) {
	s.eventBus = bus
}

// SetQuoteContactReader injects the contact reader (set after construction to break circular deps).
func (s *Service) SetQuoteContactReader(cr QuoteContactReader) {
	s.contacts = cr
}

// Create creates a new quote with line items, computing totals server-side
func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, actorID uuid.UUID, req transport.CreateQuoteRequest) (*transport.QuoteResponse, error) {
	// Generate the quote number atomically
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

	// Server-side calculation
	calcReq := transport.QuoteCalculationRequest{
		Items:         req.Items,
		PricingMode:   pricingMode,
		DiscountType:  discountType,
		DiscountValue: req.DiscountValue,
	}
	calc := CalculateQuote(calcReq)

	now := time.Now()
	quote := repository.Quote{
		ID:                  uuid.New(),
		OrganizationID:      tenantID,
		LeadID:              req.LeadID,
		LeadServiceID:       req.LeadServiceID,
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
		ValidUntil:          req.ValidUntil,
		Notes:               nilIfEmpty(req.Notes),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	items := make([]repository.QuoteItem, len(req.Items))
	for i, it := range req.Items {
		// New items: non-optional are always selected; optional items use request value
		selected := true
		if it.IsOptional {
			selected = it.IsSelected
		}
		items[i] = repository.QuoteItem{
			ID:              uuid.New(),
			QuoteID:         quote.ID,
			OrganizationID:  tenantID,
			Description:     it.Description,
			Quantity:        it.Quantity,
			QuantityNumeric: parseQuantityNumber(it.Quantity),
			UnitPriceCents:  it.UnitPriceCents,
			TaxRateBps:      it.TaxRateBps,
			IsOptional:      it.IsOptional,
			IsSelected:      selected,
			SortOrder:       i,
			CreatedAt:       now,
		}
	}

	if err := s.repo.CreateWithItems(ctx, &quote, items); err != nil {
		return nil, err
	}

	// Timeline event: "Quote OFF-2026-0001 created"
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      "quote_created",
		Title:          fmt.Sprintf("Offerte %s aangemaakt", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(quote.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId": quote.ID,
			"status":  quote.Status,
		},
	})

	return s.buildResponse(&quote, items), nil
}

// applyQuoteUpdates applies optional field updates from the request to the quote.
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
}

// buildItemsFromRequest converts request items into repository QuoteItem models.
func buildItemsFromRequest(quoteID, tenantID uuid.UUID, items []transport.QuoteItemRequest) []repository.QuoteItem {
	now := time.Now()
	result := make([]repository.QuoteItem, len(items))
	for i, it := range items {
		selected := true
		if it.IsOptional {
			selected = it.IsSelected
		}
		result[i] = repository.QuoteItem{
			ID:              uuid.New(),
			QuoteID:         quoteID,
			OrganizationID:  tenantID,
			Description:     it.Description,
			Quantity:        it.Quantity,
			QuantityNumeric: parseQuantityNumber(it.Quantity),
			UnitPriceCents:  it.UnitPriceCents,
			TaxRateBps:      it.TaxRateBps,
			IsOptional:      it.IsOptional,
			IsSelected:      selected,
			SortOrder:       i,
			CreatedAt:       now,
		}
	}
	return result
}

// toItemRequests converts repository QuoteItems to transport QuoteItemRequests.
func toItemRequests(items []repository.QuoteItem) []transport.QuoteItemRequest {
	reqs := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		reqs[i] = transport.QuoteItemRequest{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
			IsSelected:     it.IsSelected,
		}
	}
	return reqs
}

// Update updates an existing quote and recalculates totals
func (s *Service) Update(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateQuoteRequest) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	applyQuoteUpdates(quote, req)

	// Replace line items if provided, otherwise re-fetch existing
	var items []repository.QuoteItem
	if req.Items != nil {
		items = buildItemsFromRequest(quote.ID, tenantID, *req.Items)
	} else {
		items, err = s.repo.GetItemsByQuoteID(ctx, id, tenantID)
		if err != nil {
			return nil, err
		}
	}

	// Recalculate totals
	var itemReqs []transport.QuoteItemRequest
	if req.Items != nil {
		itemReqs = *req.Items
	} else {
		itemReqs = toItemRequests(items)
	}
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

	if err := s.repo.UpdateWithItems(ctx, quote, items, req.Items != nil); err != nil {
		return nil, err
	}

	return s.buildResponse(quote, items), nil
}

// GetByID retrieves a quote with its line items
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.GetItemsByQuoteID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	annotations, _ := s.repo.ListAnnotationsByQuoteID(ctx, id)

	return s.buildResponse(quote, items, annotations), nil
}

// List retrieves quotes with filtering and pagination
func (s *Service) List(ctx context.Context, tenantID uuid.UUID, req transport.ListQuotesRequest) (*transport.QuoteListResponse, error) {
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
		parsed, err := uuid.Parse(req.LeadID)
		if err != nil {
			return nil, apperr.BadRequest("invalid leadId format")
		}
		params.LeadID = &parsed
	}

	result, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	items := make([]transport.QuoteResponse, len(result.Items))
	for i, q := range result.Items {
		qItems, _ := s.repo.GetItemsByQuoteID(ctx, q.ID, tenantID)
		items[i] = *s.buildResponse(&q, qItems)
	}

	return &transport.QuoteListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

// UpdateStatus changes the status of a quote
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, actorID uuid.UUID, status transport.QuoteStatus) (*transport.QuoteResponse, error) {
	if err := s.repo.UpdateStatus(ctx, id, tenantID, string(status)); err != nil {
		return nil, err
	}

	resp, err := s.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Timeline event: "Quote OFF-2026-0001 marked Sent"
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         resp.LeadID,
		ServiceID:      resp.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      actorID.String(),
		EventType:      "quote_status_changed",
		Title:          fmt.Sprintf("Offerte %s → %s", resp.QuoteNumber, string(status)),
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(resp.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId": resp.ID,
			"status":  string(status),
		},
	})

	return resp, nil
}

// Delete removes a quote and its line items
func (s *Service) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return s.repo.Delete(ctx, id, tenantID)
}

// generatePublicToken creates a 32-byte cryptographically random hex token.
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

func isReadOnlyToken(kind repository.TokenKind) bool {
	return kind == repository.TokenKindPreview
}

func (s *Service) resolveToken(ctx context.Context, token string) (*repository.Quote, repository.TokenKind, error) {
	return s.repo.GetByToken(ctx, token)
}

// Send generates a public token for the quote and transitions it to "Sent" status.
func (s *Service) Send(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, agentID uuid.UUID) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Only draft quotes can be sent
	if quote.Status != string(transport.QuoteStatusDraft) {
		return nil, apperr.BadRequest("only draft quotes can be sent")
	}

	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days
	if quote.ValidUntil != nil && quote.ValidUntil.After(time.Now()) {
		expiresAt = *quote.ValidUntil
	}

	if err := s.repo.SetPublicToken(ctx, id, tenantID, token, expiresAt); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateStatus(ctx, id, tenantID, string(transport.QuoteStatusSent)); err != nil {
		return nil, err
	}

	// Re-fetch after updates
	resp, err := s.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Emit event
	if s.eventBus != nil {
		evt := events.QuoteSent{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        id,
			OrganizationID: tenantID,
			LeadID:         quote.LeadID,
			LeadServiceID:  quote.LeadServiceID,
			PublicToken:    token,
			QuoteNumber:    quote.QuoteNumber,
			AgentID:        agentID,
		}
		// Enrich event with contact data for email delivery
		if s.contacts != nil {
			contactData, err := s.contacts.GetQuoteContactData(ctx, quote.LeadID, tenantID)
			if err == nil {
				evt.ConsumerEmail = contactData.ConsumerEmail
				evt.ConsumerName = contactData.ConsumerName
				evt.OrganizationName = contactData.OrganizationName
			}
			// If lookup fails, we still publish the event (SSE still works) but email won't send
		}
		s.eventBus.Publish(ctx, evt)
	}

	// Timeline event
	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: tenantID,
		ActorType:      "User",
		ActorName:      agentID.String(),
		EventType:      "quote_sent",
		Title:          fmt.Sprintf("Offerte %s verstuurd", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf(msgTotalFormat, float64(quote.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId": id,
			"status":  "Sent",
		},
	})

	return resp, nil
}

// GetPublicQuoteID resolves a public token to its quote UUID (for SSE subscription).
func (s *Service) GetPublicQuoteID(ctx context.Context, token string) (uuid.UUID, error) {
	quote, _, err := s.resolveToken(ctx, token)
	if err != nil {
		return uuid.Nil, err
	}
	return quote.ID, nil
}

// GetPDFFileKey returns the PDF file key for a quote (used for public PDF downloads).
func (s *Service) GetPDFFileKey(ctx context.Context, quoteID uuid.UUID) (string, error) {
	// GetByPublicToken isn't suitable here — use a direct query.
	// We need a method that gets by ID without org scoping.
	return s.repo.GetPDFFileKeyByQuoteID(ctx, quoteID)
}

// GetOrganizationID returns the organization ID for a quote (no org scoping).
func (s *Service) GetOrganizationID(ctx context.Context, quoteID uuid.UUID) (uuid.UUID, error) {
	return s.repo.GetOrganizationIDByQuoteID(ctx, quoteID)
}

// GetPublic retrieves a quote by its public token for unauthenticated lead access.
func (s *Service) GetPublic(ctx context.Context, token string) (*transport.PublicQuoteResponse, error) {
	quote, tokenKind, err := s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}
	readOnly := isReadOnlyToken(tokenKind)

	// Check expiry
	if expAt := tokenExpiresAt(quote, tokenKind); expAt != nil && expAt.Before(time.Now()) {
		return nil, apperr.Gone(msgLinkExpired)
	}

	// Mark viewed if first time
	if !readOnly && quote.ViewedAt == nil {
		if err := s.repo.SetViewedAt(ctx, quote.ID); err != nil {
			return nil, err
		}
		now := time.Now()
		quote.ViewedAt = &now

		if s.eventBus != nil {
			s.eventBus.Publish(ctx, events.QuoteViewed{
				BaseEvent:      events.NewBaseEvent(),
				QuoteID:        quote.ID,
				OrganizationID: quote.OrganizationID,
				LeadID:         quote.LeadID,
			})
		}
	}

	items, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}

	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	return s.buildPublicResponse(quote, items, orgName, customerName, readOnly)
}

// ToggleLineItem toggles the selection of an optional line item and recalculates totals.
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

	// Recalculate totals
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
		itemReqs[i] = transport.QuoteItemRequest{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
			IsSelected:     selected,
		}
	}

	calc := CalculateQuote(transport.QuoteCalculationRequest{
		Items:         itemReqs,
		PricingMode:   quote.PricingMode,
		DiscountType:  quote.DiscountType,
		DiscountValue: quote.DiscountValue,
	})

	if err := s.repo.UpdateQuoteTotals(ctx, quote.ID, calc.SubtotalCents, calc.DiscountAmountCents, calc.VatTotalCents, calc.TotalCents); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteUpdatedByCustomer{
			BaseEvent:       events.NewBaseEvent(),
			QuoteID:         quote.ID,
			OrganizationID:  quote.OrganizationID,
			ItemID:          itemID,
			ItemDescription: item.Description,
			IsSelected:      req.IsSelected,
			NewTotalCents:   calc.TotalCents,
		})
	}

	return &transport.ToggleItemResponse{
		SubtotalCents:       calc.SubtotalCents,
		DiscountAmountCents: calc.DiscountAmountCents,
		TaxTotalCents:       calc.VatTotalCents,
		TotalCents:          calc.TotalCents,
		VatBreakdown:        calc.VatBreakdown,
	}, nil
}

// AnnotateItem adds an annotation (question/comment) to a quote line item.
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

	// Validate item belongs to quote
	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return nil, err
	}

	var authorUUID *uuid.UUID
	if authorID != "" {
		parsed, parseErr := uuid.Parse(authorID)
		if parseErr == nil {
			authorUUID = &parsed
		}
	}

	annotation := repository.QuoteAnnotation{
		ID:             uuid.New(),
		QuoteItemID:    itemID,
		OrganizationID: quote.OrganizationID,
		AuthorType:     authorType,
		AuthorID:       authorUUID,
		Text:           text,
		IsResolved:     false,
		CreatedAt:      time.Now(),
	}

	if err := s.repo.CreateAnnotation(ctx, &annotation); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteAnnotated{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        quote.ID,
			OrganizationID: quote.OrganizationID,
			ItemID:         itemID,
			AuthorType:     authorType,
			AuthorID:       authorID,
			Text:           text,
		})
	}

	return &transport.AnnotationResponse{
		ID:         annotation.ID,
		ItemID:     annotation.QuoteItemID,
		AuthorType: annotation.AuthorType,
		AuthorID:   annotation.AuthorID,
		Text:       annotation.Text,
		IsResolved: annotation.IsResolved,
		CreatedAt:  annotation.CreatedAt,
	}, nil
}

// UpdateAnnotation updates an existing annotation's text (public flow).
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

	return &transport.AnnotationResponse{
		ID:         annotation.ID,
		ItemID:     annotation.QuoteItemID,
		AuthorType: annotation.AuthorType,
		AuthorID:   annotation.AuthorID,
		Text:       annotation.Text,
		IsResolved: annotation.IsResolved,
		CreatedAt:  annotation.CreatedAt,
	}, nil
}

// DeleteAnnotation removes an existing annotation if it has no agent response.
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

// AgentAnnotateItem lets an authenticated agent add an annotation to a quote item.
func (s *Service) AgentAnnotateItem(ctx context.Context, quoteID, itemID, tenantID, agentID uuid.UUID, text string) (*transport.AnnotationResponse, error) {
	quote, err := s.repo.GetByID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.GetItemByID(ctx, itemID, quote.ID); err != nil {
		return nil, err
	}

	agentPtr := &agentID
	annotation := repository.QuoteAnnotation{
		ID:             uuid.New(),
		QuoteItemID:    itemID,
		OrganizationID: tenantID,
		AuthorType:     "agent",
		AuthorID:       agentPtr,
		Text:           text,
		IsResolved:     false,
		CreatedAt:      time.Now(),
	}

	if err := s.repo.CreateAnnotation(ctx, &annotation); err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteAnnotated{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        quoteID,
			OrganizationID: tenantID,
			ItemID:         itemID,
			AuthorType:     "agent",
			AuthorID:       agentID.String(),
			Text:           text,
		})
	}

	return &transport.AnnotationResponse{
		ID:         annotation.ID,
		ItemID:     annotation.QuoteItemID,
		AuthorType: annotation.AuthorType,
		AuthorID:   annotation.AuthorID,
		Text:       annotation.Text,
		IsResolved: annotation.IsResolved,
		CreatedAt:  annotation.CreatedAt,
	}, nil
}

// Accept processes a lead accepting the quote with their digital signature.
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

	// Refresh
	quote, _, err = s.resolveToken(ctx, token)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.GetItemsByQuoteIDNoOrg(ctx, quote.ID)
	if err != nil {
		return nil, err
	}

	if s.eventBus != nil {
		evt := events.QuoteAccepted{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        quote.ID,
			OrganizationID: quote.OrganizationID,
			LeadID:         quote.LeadID,
			LeadServiceID:  quote.LeadServiceID,
			SignatureName:  req.SignatureName,
			TotalCents:     quote.TotalCents,
			QuoteNumber:    quote.QuoteNumber,
		}
		// Enrich event with contact + agent data for email delivery and PDF
		if s.contacts != nil {
			contactData, lookupErr := s.contacts.GetQuoteContactData(ctx, quote.LeadID, quote.OrganizationID)
			if lookupErr == nil {
				evt.ConsumerEmail = contactData.ConsumerEmail
				evt.ConsumerName = contactData.ConsumerName
				evt.OrganizationName = contactData.OrganizationName
				evt.AgentEmail = contactData.AgentEmail
				evt.AgentName = contactData.AgentName
			}
		}
		s.eventBus.Publish(ctx, evt)
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: quote.OrganizationID,
		ActorType:      "Lead",
		ActorName:      req.SignatureName,
		EventType:      "quote_accepted",
		Title:          fmt.Sprintf("Offerte %s geaccepteerd", quote.QuoteNumber),
		Summary:        toPtr(fmt.Sprintf("Ondertekend door %s — "+msgTotalFormat, req.SignatureName, float64(quote.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId":       quote.ID,
			"status":        "Accepted",
			"signatureName": req.SignatureName,
		},
	})

	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	return s.buildPublicResponse(quote, items, orgName, customerName, false)
}

// Reject processes a lead rejecting the quote.
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

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.QuoteRejected{
			BaseEvent:      events.NewBaseEvent(),
			QuoteID:        quote.ID,
			OrganizationID: quote.OrganizationID,
			LeadID:         quote.LeadID,
			LeadServiceID:  quote.LeadServiceID,
			Reason:         req.Reason,
		})
	}

	s.emitTimelineEvent(ctx, TimelineEventParams{
		LeadID:         quote.LeadID,
		ServiceID:      quote.LeadServiceID,
		OrganizationID: quote.OrganizationID,
		ActorType:      "Lead",
		ActorName:      "Klant",
		EventType:      "quote_rejected",
		Title:          fmt.Sprintf("Offerte %s afgewezen", quote.QuoteNumber),
		Summary:        nilIfEmpty(req.Reason),
		Metadata: map[string]any{
			"quoteId": quote.ID,
			"status":  "Rejected",
			"reason":  req.Reason,
		},
	})

	orgName, customerName := s.lookupContactNames(ctx, quote.LeadID, quote.OrganizationID)
	return s.buildPublicResponse(quote, items, orgName, customerName, false)
}

// buildPublicResponse converts a repository Quote + items into a public transport response.
func (s *Service) buildPublicResponse(q *repository.Quote, items []repository.QuoteItem, organizationName, customerName string, readOnly bool) (*transport.PublicQuoteResponse, error) {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	annotations, _ := s.repo.ListAnnotationsByQuoteID(context.Background(), q.ID)
	annotationsByItem := make(map[uuid.UUID][]transport.AnnotationResponse)
	for _, ann := range annotations {
		annotationsByItem[ann.QuoteItemID] = append(annotationsByItem[ann.QuoteItemID], transport.AnnotationResponse{
			ID:         ann.ID,
			ItemID:     ann.QuoteItemID,
			AuthorType: ann.AuthorType,
			AuthorID:   ann.AuthorID,
			Text:       ann.Text,
			IsResolved: ann.IsResolved,
			CreatedAt:  ann.CreatedAt,
		})
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

		respItems[i] = transport.PublicQuoteItemResponse{
			ID:                  it.ID,
			Description:         it.Description,
			Quantity:            it.Quantity,
			UnitPriceCents:      it.UnitPriceCents,
			TaxRateBps:          it.TaxRateBps,
			IsOptional:          it.IsOptional,
			IsSelected:          it.IsSelected,
			SortOrder:           it.SortOrder,
			TotalBeforeTaxCents: roundCents(lineSubtotal),
			TotalTaxCents:       roundCents(lineVat),
			LineTotalCents:      roundCents(lineSubtotal + lineVat),
			Annotations:         annotationsByItem[it.ID],
		}
		if respItems[i].Annotations == nil {
			respItems[i].Annotations = []transport.AnnotationResponse{}
		}
	}

	// Compute VAT breakdown
	itemReqs := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		itemReqs[i] = transport.QuoteItemRequest{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
			IsSelected:     it.IsSelected,
		}
	}
	calc := CalculateQuote(transport.QuoteCalculationRequest{
		Items:         itemReqs,
		PricingMode:   q.PricingMode,
		DiscountType:  q.DiscountType,
		DiscountValue: q.DiscountValue,
	})

	return &transport.PublicQuoteResponse{
		ID:                  q.ID,
		QuoteNumber:         q.QuoteNumber,
		Status:              transport.QuoteStatus(q.Status),
		PricingMode:         q.PricingMode,
		OrganizationName:    organizationName,
		CustomerName:        customerName,
		DiscountType:        q.DiscountType,
		DiscountValue:       q.DiscountValue,
		SubtotalCents:       calc.SubtotalCents,
		DiscountAmountCents: calc.DiscountAmountCents,
		TaxTotalCents:       calc.VatTotalCents,
		TotalCents:          calc.TotalCents,
		VatBreakdown:        calc.VatBreakdown,
		ValidUntil:          q.ValidUntil,
		Notes:               q.Notes,
		Items:               respItems,
		AcceptedAt:          q.AcceptedAt,
		RejectedAt:          q.RejectedAt,
		IsReadOnly:          readOnly,
	}, nil
}

// GetPreviewLink returns (or creates) a read-only preview token for a quote.
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
		return &transport.QuotePreviewLinkResponse{
			Token:     *quote.PreviewToken,
			ExpiresAt: quote.PreviewTokenExpAt,
		}, nil
	}

	token, err := generatePublicToken()
	if err != nil {
		return nil, err
	}

	expiresAt := now.Add(30 * 24 * time.Hour)
	if quote.ValidUntil != nil && quote.ValidUntil.After(now) {
		expiresAt = *quote.ValidUntil
	}

	if err := s.repo.SetPreviewToken(ctx, id, tenantID, token, expiresAt); err != nil {
		return nil, err
	}

	return &transport.QuotePreviewLinkResponse{
		Token:     token,
		ExpiresAt: &expiresAt,
	}, nil
}

// buildResponse converts a repository Quote + items into a transport response
func (s *Service) buildResponse(q *repository.Quote, items []repository.QuoteItem, annotations ...[]repository.QuoteAnnotation) *transport.QuoteResponse {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	// Index annotations by item ID
	annotationsByItem := make(map[uuid.UUID][]transport.AnnotationResponse)
	if len(annotations) > 0 {
		for _, a := range annotations[0] {
			annotationsByItem[a.QuoteItemID] = append(annotationsByItem[a.QuoteItemID], transport.AnnotationResponse{
				ID:         a.ID,
				ItemID:     a.QuoteItemID,
				AuthorType: a.AuthorType,
				AuthorID:   a.AuthorID,
				Text:       a.Text,
				IsResolved: a.IsResolved,
				CreatedAt:  a.CreatedAt,
			})
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

		respItems[i] = transport.QuoteItemResponse{
			ID:                  it.ID,
			Description:         it.Description,
			Quantity:            it.Quantity,
			UnitPriceCents:      it.UnitPriceCents,
			TaxRateBps:          it.TaxRateBps,
			IsOptional:          it.IsOptional,
			IsSelected:          it.IsSelected,
			SortOrder:           it.SortOrder,
			TotalBeforeTaxCents: roundCents(lineSubtotal),
			TotalTaxCents:       roundCents(lineVat),
			LineTotalCents:      roundCents(lineSubtotal + lineVat),
			Annotations:         annotationsByItem[it.ID],
		}
		if respItems[i].Annotations == nil {
			respItems[i].Annotations = []transport.AnnotationResponse{}
		}
	}

	return &transport.QuoteResponse{
		ID:                         q.ID,
		QuoteNumber:                q.QuoteNumber,
		LeadID:                     q.LeadID,
		LeadServiceID:              q.LeadServiceID,
		CreatedByID:                q.CreatedByID,
		CreatedByFirstName:         q.CreatedByFirstName,
		CreatedByLastName:          q.CreatedByLastName,
		CreatedByEmail:             q.CreatedByEmail,
		CustomerFirstName:          q.CustomerFirstName,
		CustomerLastName:           q.CustomerLastName,
		CustomerPhone:              q.CustomerPhone,
		CustomerEmail:              q.CustomerEmail,
		CustomerAddressStreet:      q.CustomerAddressStreet,
		CustomerAddressHouseNumber: q.CustomerAddressHouseNumber,
		CustomerAddressZipCode:     q.CustomerAddressZipCode,
		CustomerAddressCity:        q.CustomerAddressCity,
		Status:                     transport.QuoteStatus(q.Status),
		PricingMode:                q.PricingMode,
		DiscountType:               q.DiscountType,
		DiscountValue:              q.DiscountValue,
		SubtotalCents:              q.SubtotalCents,
		DiscountAmountCents:        q.DiscountAmountCents,
		TaxTotalCents:              q.TaxTotalCents,
		TotalCents:                 q.TotalCents,
		ValidUntil:                 q.ValidUntil,
		Notes:                      q.Notes,
		Items:                      respItems,
		ViewedAt:                   q.ViewedAt,
		AcceptedAt:                 q.AcceptedAt,
		RejectedAt:                 q.RejectedAt,
		PDFFileKey:                 q.PDFFileKey,
		CreatedAt:                  q.CreatedAt,
		UpdatedAt:                  q.UpdatedAt,
	}
}

// emitTimelineEvent fires a timeline event if a TimelineWriter is configured.
// Failures are logged but never block the main flow.
func (s *Service) emitTimelineEvent(ctx context.Context, params TimelineEventParams) {
	if s.timeline == nil {
		return
	}
	// Best-effort — do not fail the request if the timeline write fails
	_ = s.timeline.CreateTimelineEvent(ctx, params)
}

func toPtr(s string) *string { return &s }

// lookupContactNames returns the organization name and customer name for a quote.
// Returns empty strings if the contact reader is unavailable or lookup fails.
func (s *Service) lookupContactNames(ctx context.Context, leadID, orgID uuid.UUID) (orgName, customerName string) {
	if s.contacts == nil {
		return "", ""
	}
	data, err := s.contacts.GetQuoteContactData(ctx, leadID, orgID)
	if err != nil {
		return "", ""
	}
	return data.OrganizationName, data.ConsumerName
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

// ListActivities returns the persisted activity log for a quote.
func (s *Service) ListActivities(ctx context.Context, quoteID, tenantID uuid.UUID) ([]transport.QuoteActivityResponse, error) {
	activities, err := s.repo.ListActivities(ctx, quoteID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list quote activities: %w", err)
	}

	out := make([]transport.QuoteActivityResponse, len(activities))
	for i, a := range activities {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		out[i] = transport.QuoteActivityResponse{
			ID:        a.ID,
			EventType: a.EventType,
			Message:   a.Message,
			Metadata:  meta,
			CreatedAt: a.CreatedAt,
		}
	}
	return out, nil
}
