package adapters

import (
	"context"
	"math"
	"strings"

	authsvc "portal_final_backend/internal/auth/service"
	leadsrepo "portal_final_backend/internal/leads/repository"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/ports"
	quotesrepo "portal_final_backend/internal/quotes/repository"
	quotesvc "portal_final_backend/internal/quotes/service"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const maxQuoteScopeHighlights = 5

// QuotePublicAdapter exposes quote data for the public lead portal.
type QuotePublicAdapter struct {
	svc                   *quotesvc.Service
	acceptedQuoteIDReader leadsrepo.QuotePriceReader
	quotesRepo            *quotesrepo.Repository
}

type quoteContentSummary struct {
	scopeItems []string
	lineItems  []ports.PublicQuoteLineItemSummary
}

func NewQuotePublicAdapter(svc *quotesvc.Service, acceptedQuoteIDReader leadsrepo.QuotePriceReader, quotesRepo *quotesrepo.Repository) *QuotePublicAdapter {
	return &QuotePublicAdapter{svc: svc, acceptedQuoteIDReader: acceptedQuoteIDReader, quotesRepo: quotesRepo}
}

func (a *QuotePublicAdapter) GetActiveQuote(ctx context.Context, leadID, orgID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	if a == nil || a.svc == nil {
		return nil, nil
	}
	quote, err := a.svc.GetLatestNonDraftByLead(ctx, leadID, orgID)
	if err != nil || quote == nil {
		return nil, err
	}
	return a.buildPublicQuoteSummary(ctx, quote), nil
}

func (a *QuotePublicAdapter) GetActiveQuoteForService(ctx context.Context, leadServiceID, orgID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	if a == nil || a.svc == nil {
		return nil, nil
	}
	quote, err := a.svc.GetLatestNonDraftByLeadService(ctx, leadServiceID, orgID)
	if err != nil || quote == nil {
		return nil, err
	}
	return a.buildPublicQuoteSummary(ctx, quote), nil
}

func (a *QuotePublicAdapter) GetAcceptedQuote(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	if a == nil || a.acceptedQuoteIDReader == nil || a.quotesRepo == nil {
		return nil, nil
	}

	quoteID, err := a.acceptedQuoteIDReader.GetLatestAcceptedQuoteIDForService(ctx, leadServiceID, organizationID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, err
	}

	quote, err := a.quotesRepo.GetByID(ctx, quoteID, organizationID)
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if quote == nil {
		return nil, nil
	}

	return a.buildPublicQuoteSummary(ctx, quote), nil
}

func (a *QuotePublicAdapter) buildPublicQuoteSummary(ctx context.Context, quote *quotesrepo.Quote) *ports.PublicQuoteSummary {
	publicToken := ""
	if quote.PublicToken != nil {
		publicToken = *quote.PublicToken
	}
	createdAt := &quote.CreatedAt

	content := a.buildQuoteContentSummary(ctx, quote)

	return &ports.PublicQuoteSummary{
		ID:          quote.ID,
		QuoteNumber: quote.QuoteNumber,
		Status:      quote.Status,
		PublicToken: publicToken,
		TotalCents:  quote.TotalCents,
		PDFFileKey:  quote.PDFFileKey,
		AcceptedAt:  quote.AcceptedAt,
		ValidUntil:  quote.ValidUntil,
		Notes:       quote.Notes,
		CreatedAt:   createdAt,
		ScopeItems:  content.scopeItems,
		LineItems:   content.lineItems,
	}
}

func (a *QuotePublicAdapter) buildQuoteContentSummary(ctx context.Context, quote *quotesrepo.Quote) quoteContentSummary {
	if a == nil || a.quotesRepo == nil || quote == nil {
		return quoteContentSummary{}
	}
	items, err := a.quotesRepo.GetItemsByQuoteID(ctx, quote.ID, quote.OrganizationID)
	if err != nil {
		return quoteContentSummary{}
	}
	highlights := make([]string, 0, maxQuoteScopeHighlights)
	highlights = appendPreferredQuoteHighlights(highlights, items)
	if len(highlights) < maxQuoteScopeHighlights {
		highlights = appendFallbackQuoteHighlights(highlights, items)
	}
	return quoteContentSummary{
		scopeItems: highlights,
		lineItems:  buildQuoteLineItemSummaries(items),
	}
}

func buildQuoteLineItemSummaries(items []quotesrepo.QuoteItem) []ports.PublicQuoteLineItemSummary {
	selected := make([]ports.PublicQuoteLineItemSummary, 0, maxQuoteScopeHighlights)
	fallback := make([]ports.PublicQuoteLineItemSummary, 0, maxQuoteScopeHighlights)
	for _, item := range items {
		summary, ok := toQuoteLineItemSummary(item)
		if !ok {
			continue
		}
		fallback = append(fallback, summary)
		if !item.IsOptional || item.IsSelected {
			selected = append(selected, summary)
		}
	}
	if len(selected) > maxQuoteScopeHighlights {
		return selected[:maxQuoteScopeHighlights]
	}
	if len(selected) > 0 {
		return selected
	}
	if len(fallback) > maxQuoteScopeHighlights {
		return fallback[:maxQuoteScopeHighlights]
	}
	return fallback
}

func toQuoteLineItemSummary(item quotesrepo.QuoteItem) (ports.PublicQuoteLineItemSummary, bool) {
	title := sanitizeReplySummary(item.Title, 100)
	description := sanitizeReplySummary(item.Description, 160)
	quantity := sanitizeReplySummary(item.Quantity, 32)
	if title == "" && description == "" {
		return ports.PublicQuoteLineItemSummary{}, false
	}
	return ports.PublicQuoteLineItemSummary{
		Title:          title,
		Description:    description,
		Quantity:       quantity,
		LineTotalCents: deriveQuoteLineTotalCents(item),
		IsOptional:     item.IsOptional,
		IsSelected:     item.IsSelected,
	}, true
}

func deriveQuoteLineTotalCents(item quotesrepo.QuoteItem) int64 {
	quantity := item.QuantityNumeric
	if quantity <= 0 {
		quantity = 1
	}
	return int64(math.Round(quantity * float64(item.UnitPriceCents)))
}

func appendPreferredQuoteHighlights(highlights []string, items []quotesrepo.QuoteItem) []string {
	for _, item := range items {
		if item.IsOptional && !item.IsSelected {
			continue
		}
		highlights = appendQuoteHighlight(highlights, item)
		if len(highlights) >= maxQuoteScopeHighlights {
			return highlights
		}
	}
	return highlights
}

func appendFallbackQuoteHighlights(highlights []string, items []quotesrepo.QuoteItem) []string {
	for _, item := range items {
		highlights = appendQuoteHighlight(highlights, item)
		if len(highlights) >= maxQuoteScopeHighlights {
			return highlights
		}
	}
	return highlights
}

func appendQuoteHighlight(highlights []string, item quotesrepo.QuoteItem) []string {
	title := sanitizeReplySummary(item.Title, 100)
	if title == "" || containsString(highlights, title) {
		return highlights
	}
	description := sanitizeReplySummary(item.Description, 120)
	if description != "" && description != title {
		title += ": " + description
	}
	return append(highlights, title)
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func sanitizeReplySummary(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > maxLen {
		return trimmed[:maxLen] + "..."
	}
	return trimmed
}

// AppointmentPublicAdapter exposes appointment data for the public lead portal.
type AppointmentPublicAdapter struct {
	svc *service.Service
}

func NewAppointmentPublicAdapter(svc *service.Service) *AppointmentPublicAdapter {
	return &AppointmentPublicAdapter{svc: svc}
}

func (a *AppointmentPublicAdapter) GetUpcomingVisit(ctx context.Context, leadID, orgID uuid.UUID) (*ports.PublicAppointmentSummary, error) {
	appt, err := a.svc.GetNextScheduledVisit(ctx, leadID, orgID)
	if err != nil || appt == nil {
		return nil, err
	}

	return &ports.PublicAppointmentSummary{
		ID:             appt.ID,
		StartTime:      appt.StartTime,
		EndTime:        appt.EndTime,
		Title:          appt.Title,
		Status:         appt.Status,
		Type:           appt.Type,
		Description:    appt.Description,
		Location:       appt.Location,
		MeetingLink:    appt.MeetingLink,
		AssignedUserID: userIDPtr(appt.UserID),
	}, nil
}

func (a *AppointmentPublicAdapter) GetPendingVisit(ctx context.Context, leadID, orgID uuid.UUID) (*ports.PublicAppointmentSummary, error) {
	appt, err := a.svc.GetNextRequestedVisit(ctx, leadID, orgID)
	if err != nil || appt == nil {
		return nil, err
	}

	return &ports.PublicAppointmentSummary{
		ID:             appt.ID,
		StartTime:      appt.StartTime,
		EndTime:        appt.EndTime,
		Title:          appt.Title,
		Status:         appt.Status,
		Type:           appt.Type,
		Description:    appt.Description,
		Location:       appt.Location,
		MeetingLink:    appt.MeetingLink,
		AssignedUserID: userIDPtr(appt.UserID),
	}, nil
}

func (a *AppointmentPublicAdapter) ListVisits(ctx context.Context, leadID, orgID uuid.UUID) ([]ports.PublicAppointmentSummary, error) {
	visits, err := a.svc.ListLeadVisitsByStatus(ctx, leadID, orgID, []transport.AppointmentStatus{
		transport.AppointmentStatusRequested,
		transport.AppointmentStatusScheduled,
	})
	if err != nil {
		return nil, err
	}

	items := make([]ports.PublicAppointmentSummary, 0, len(visits))
	for _, appt := range visits {
		items = append(items, ports.PublicAppointmentSummary{
			ID:             appt.ID,
			StartTime:      appt.StartTime,
			EndTime:        appt.EndTime,
			Title:          appt.Title,
			Status:         appt.Status,
			Type:           appt.Type,
			Description:    appt.Description,
			Location:       appt.Location,
			MeetingLink:    appt.MeetingLink,
			AssignedUserID: userIDPtr(appt.UserID),
		})
	}

	return items, nil
}

func userIDPtr(userID uuid.UUID) *uuid.UUID {
	if userID == uuid.Nil {
		return nil
	}
	value := userID
	return &value
}

type ReplyUserReaderAdapter struct {
	svc *authsvc.Service
}

func NewReplyUserReaderAdapter(svc *authsvc.Service) *ReplyUserReaderAdapter {
	return &ReplyUserReaderAdapter{svc: svc}
}

func (a *ReplyUserReaderAdapter) GetUserProfile(ctx context.Context, userID uuid.UUID) (*ports.ReplyUserProfile, error) {
	if a == nil || a.svc == nil || userID == uuid.Nil {
		return nil, nil
	}

	profile, err := a.svc.GetMe(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ports.ReplyUserProfile{
		ID:        profile.ID,
		Email:     profile.Email,
		FirstName: profile.FirstName,
		LastName:  profile.LastName,
	}, nil
}

var _ ports.QuotePublicViewer = (*QuotePublicAdapter)(nil)
var _ ports.ReplyQuoteReader = (*QuotePublicAdapter)(nil)
var _ ports.AppointmentPublicViewer = (*AppointmentPublicAdapter)(nil)
var _ ports.ReplyUserReader = (*ReplyUserReaderAdapter)(nil)
