package adapters

import (
	"context"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/leads/ports"
	quotesvc "portal_final_backend/internal/quotes/service"

	"github.com/google/uuid"
)

// QuotePublicAdapter exposes quote data for the public lead portal.
type QuotePublicAdapter struct {
	svc *quotesvc.Service
}

func NewQuotePublicAdapter(svc *quotesvc.Service) *QuotePublicAdapter {
	return &QuotePublicAdapter{svc: svc}
}

func (a *QuotePublicAdapter) GetActiveQuote(ctx context.Context, leadID, orgID uuid.UUID) (*ports.PublicQuoteSummary, error) {
	quote, err := a.svc.GetLatestNonDraftByLead(ctx, leadID, orgID)
	if err != nil || quote == nil {
		return nil, err
	}

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
	}, nil
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
	}, nil
}

var _ ports.QuotePublicViewer = (*QuotePublicAdapter)(nil)
var _ ports.AppointmentPublicViewer = (*AppointmentPublicAdapter)(nil)
