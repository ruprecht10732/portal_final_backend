package service

import (
	"context"
	"fmt"
	"time"

	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
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

// Service provides business logic for quotes
type Service struct {
	repo     *repository.Repository
	timeline TimelineWriter // optional — nil means no timeline integration
}

// New creates a new quotes service
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// SetTimelineWriter injects the timeline writer (set after construction to break circular deps).
func (s *Service) SetTimelineWriter(tw TimelineWriter) {
	s.timeline = tw
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
		Summary:        toPtr(fmt.Sprintf("Totaal: €%.2f", float64(quote.TotalCents)/100)),
		Metadata: map[string]any{
			"quoteId": quote.ID,
			"status":  quote.Status,
		},
	})

	return s.buildResponse(&quote, items), nil
}

// Update updates an existing quote and recalculates totals
func (s *Service) Update(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateQuoteRequest) (*transport.QuoteResponse, error) {
	quote, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Apply updates
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

	// Replace line items if provided
	var items []repository.QuoteItem
	if req.Items != nil {
		now := time.Now()
		items = make([]repository.QuoteItem, len(*req.Items))
		for i, it := range *req.Items {
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
				SortOrder:       i,
				CreatedAt:       now,
			}
		}

		// Recalculate totals
		calcReq := transport.QuoteCalculationRequest{
			Items:         *req.Items,
			PricingMode:   quote.PricingMode,
			DiscountType:  quote.DiscountType,
			DiscountValue: quote.DiscountValue,
		}
		calc := CalculateQuote(calcReq)
		quote.SubtotalCents = calc.SubtotalCents
		quote.DiscountAmountCents = calc.DiscountAmountCents
		quote.TaxTotalCents = calc.VatTotalCents
		quote.TotalCents = calc.TotalCents
	} else {
		// Re-fetch existing items to recalculate with updated discount
		existingItems, err := s.repo.GetItemsByQuoteID(ctx, id, tenantID)
		if err != nil {
			return nil, err
		}
		items = existingItems

		// Recalculate totals with existing items
		itemReqs := make([]transport.QuoteItemRequest, len(existingItems))
		for i, it := range existingItems {
			itemReqs[i] = transport.QuoteItemRequest{
				Description:    it.Description,
				Quantity:       it.Quantity,
				UnitPriceCents: it.UnitPriceCents,
				TaxRateBps:     it.TaxRateBps,
				IsOptional:     it.IsOptional,
			}
		}
		calcReq := transport.QuoteCalculationRequest{
			Items:         itemReqs,
			PricingMode:   quote.PricingMode,
			DiscountType:  quote.DiscountType,
			DiscountValue: quote.DiscountValue,
		}
		calc := CalculateQuote(calcReq)
		quote.SubtotalCents = calc.SubtotalCents
		quote.DiscountAmountCents = calc.DiscountAmountCents
		quote.TaxTotalCents = calc.VatTotalCents
		quote.TotalCents = calc.TotalCents
	}

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

	return s.buildResponse(quote, items), nil
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
		Summary:        toPtr(fmt.Sprintf("Totaal: €%.2f", float64(resp.TotalCents)/100)),
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

// buildResponse converts a repository Quote + items into a transport response
func (s *Service) buildResponse(q *repository.Quote, items []repository.QuoteItem) *transport.QuoteResponse {
	pricingMode := q.PricingMode
	if pricingMode == "" {
		pricingMode = "exclusive"
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
			SortOrder:           it.SortOrder,
			TotalBeforeTaxCents: roundCents(lineSubtotal),
			TotalTaxCents:       roundCents(lineVat),
			LineTotalCents:      roundCents(lineSubtotal + lineVat),
		}
	}

	return &transport.QuoteResponse{
		ID:                  q.ID,
		QuoteNumber:         q.QuoteNumber,
		LeadID:              q.LeadID,
		LeadServiceID:       q.LeadServiceID,
		Status:              transport.QuoteStatus(q.Status),
		PricingMode:         q.PricingMode,
		DiscountType:        q.DiscountType,
		DiscountValue:       q.DiscountValue,
		SubtotalCents:       q.SubtotalCents,
		DiscountAmountCents: q.DiscountAmountCents,
		TaxTotalCents:       q.TaxTotalCents,
		TotalCents:          q.TotalCents,
		ValidUntil:          q.ValidUntil,
		Notes:               q.Notes,
		Items:               respItems,
		CreatedAt:           q.CreatedAt,
		UpdatedAt:           q.UpdatedAt,
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
