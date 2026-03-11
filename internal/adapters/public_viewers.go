package adapters

import (
	"context"

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

// QuotePublicAdapter exposes quote data for the public lead portal.
type QuotePublicAdapter struct {
	svc                   *quotesvc.Service
	acceptedQuoteIDReader leadsrepo.QuotePriceReader
	quotesRepo            *quotesrepo.Repository
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
	return buildPublicQuoteSummary(quote), nil
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

	return buildPublicQuoteSummary(quote), nil
}

func buildPublicQuoteSummary(quote *quotesrepo.Quote) *ports.PublicQuoteSummary {
	publicToken := ""
	if quote.PublicToken != nil {
		publicToken = *quote.PublicToken
	}

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
	}
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
		ID:        appt.ID,
		StartTime: appt.StartTime,
		EndTime:   appt.EndTime,
		Title:     appt.Title,
		Status:    appt.Status,
	}, nil
}

func (a *AppointmentPublicAdapter) GetPendingVisit(ctx context.Context, leadID, orgID uuid.UUID) (*ports.PublicAppointmentSummary, error) {
	appt, err := a.svc.GetNextRequestedVisit(ctx, leadID, orgID)
	if err != nil || appt == nil {
		return nil, err
	}

	return &ports.PublicAppointmentSummary{
		ID:        appt.ID,
		StartTime: appt.StartTime,
		EndTime:   appt.EndTime,
		Title:     appt.Title,
		Status:    appt.Status,
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
			ID:        appt.ID,
			StartTime: appt.StartTime,
			EndTime:   appt.EndTime,
			Title:     appt.Title,
			Status:    appt.Status,
		})
	}

	return items, nil
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
