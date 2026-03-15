package adapters

import (
	"context"

	appointmentsservice "portal_final_backend/internal/appointments/service"
	appointmentstransport "portal_final_backend/internal/appointments/transport"
	quotesservice "portal_final_backend/internal/quotes/service"
	quotestransport "portal_final_backend/internal/quotes/transport"

	"github.com/google/uuid"
)

type LeadDetailQuoteReader struct {
	svc *quotesservice.Service
}

func NewLeadDetailQuoteReader(svc *quotesservice.Service) *LeadDetailQuoteReader {
	return &LeadDetailQuoteReader{svc: svc}
}

func (r *LeadDetailQuoteReader) ListLeadQuotes(ctx context.Context, tenantID uuid.UUID, leadID uuid.UUID) ([]quotestransport.QuoteResponse, error) {
	if r == nil || r.svc == nil {
		return []quotestransport.QuoteResponse{}, nil
	}
	response, err := r.svc.List(ctx, tenantID, quotestransport.ListQuotesRequest{
		LeadID:    leadID.String(),
		Page:      1,
		PageSize:  100,
		SortBy:    "createdAt",
		SortOrder: "desc",
	})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

type LeadDetailAppointmentReader struct {
	svc *appointmentsservice.Service
}

func NewLeadDetailAppointmentReader(svc *appointmentsservice.Service) *LeadDetailAppointmentReader {
	return &LeadDetailAppointmentReader{svc: svc}
}

func (r *LeadDetailAppointmentReader) ListLeadAppointments(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, leadID uuid.UUID) ([]appointmentstransport.AppointmentResponse, error) {
	if r == nil || r.svc == nil {
		return []appointmentstransport.AppointmentResponse{}, nil
	}
	appointmentType := appointmentstransport.AppointmentTypeLeadVisit
	response, err := r.svc.List(ctx, userID, isAdmin, tenantID, appointmentstransport.ListAppointmentsRequest{
		LeadID:   leadID.String(),
		Type:     &appointmentType,
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}
