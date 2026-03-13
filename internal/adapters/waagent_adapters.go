package adapters

import (
	"context"
	"log"
	"strings"
	"time"

	"portal_final_backend/internal/appointments/service"
	appointmenttransport "portal_final_backend/internal/appointments/transport"
	quotesvc "portal_final_backend/internal/quotes/service"
	quotetransport "portal_final_backend/internal/quotes/transport"
	"portal_final_backend/internal/waagent"

	"github.com/google/uuid"
)

// WAAgentQuotesAdapter adapts the quotes service for the waagent module.
type WAAgentQuotesAdapter struct {
	svc *quotesvc.Service
}

// NewWAAgentQuotesAdapter creates a quotes reader for the waagent.
func NewWAAgentQuotesAdapter(svc *quotesvc.Service) *WAAgentQuotesAdapter {
	return &WAAgentQuotesAdapter{svc: svc}
}

func (a *WAAgentQuotesAdapter) ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]waagent.QuoteSummary, error) {
	req := quotetransport.ListQuotesRequest{PageSize: 20}
	if status != nil {
		req.Status = *status
	}
	resp, err := a.svc.List(ctx, orgID, req)
	if err != nil {
		log.Printf("waagent: ListQuotes error org=%s: %v", orgID, err)
		return nil, err
	}
	log.Printf("waagent: ListQuotes org=%s status=%v items=%d", orgID, status, len(resp.Items))
	out := make([]waagent.QuoteSummary, 0, len(resp.Items))
	for _, q := range resp.Items {
		clientName := ""
		if q.CustomerFirstName != nil {
			clientName = *q.CustomerFirstName
		}
		if q.CustomerLastName != nil {
			if clientName != "" {
				clientName += " "
			}
			clientName += *q.CustomerLastName
		}
		out = append(out, waagent.QuoteSummary{
			QuoteID:       q.ID.String(),
			LeadID:        q.LeadID.String(),
			LeadServiceID: uuidPtrToString(q.LeadServiceID),
			QuoteNumber: q.QuoteNumber,
			ClientName:  clientName,
			ClientPhone: derefString(q.CustomerPhone),
			ClientEmail: derefString(q.CustomerEmail),
			ClientCity:  derefString(q.CustomerAddressCity),
			TotalCents:  q.TotalCents,
			Status:      string(q.Status),
			Summary:     summarizeQuote(q),
			CreatedAt:   q.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func summarizeQuote(q quotetransport.QuoteResponse) string {
	parts := make([]string, 0, len(q.Items))
	for _, item := range q.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.Description)
		}
		if title == "" {
			continue
		}
		parts = append(parts, title)
		if len(parts) >= 3 {
			break
		}
	}

	summary := strings.Join(parts, ", ")
	if notes := strings.TrimSpace(valueOrEmpty(q.Notes)); notes != "" {
		if summary == "" {
			summary = notes
		} else {
			summary += ". " + notes
		}
	}
	return strings.TrimSpace(summary)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// WAAgentAppointmentsAdapter adapts the appointments service for the waagent module.
type WAAgentAppointmentsAdapter struct {
	svc *service.Service
}

// NewWAAgentAppointmentsAdapter creates an appointments reader for the waagent.
func NewWAAgentAppointmentsAdapter(svc *service.Service) *WAAgentAppointmentsAdapter {
	return &WAAgentAppointmentsAdapter{svc: svc}
}

func (a *WAAgentAppointmentsAdapter) ListAppointmentsByOrganization(ctx context.Context, orgID uuid.UUID, from, to *time.Time) ([]waagent.AppointmentSummary, error) {
	req := appointmenttransport.ListAppointmentsRequest{PageSize: 20}
	if from != nil {
		req.StartFrom = from.Format("2006-01-02")
	}
	if to != nil {
		req.StartTo = to.Format("2006-01-02")
	}
	resp, err := a.svc.List(ctx, uuid.Nil, true, orgID, req)
	if err != nil {
		log.Printf("waagent: ListAppointments error org=%s: %v", orgID, err)
		return nil, err
	}
	log.Printf("waagent: ListAppointments org=%s items=%d", orgID, len(resp.Items))
	out := make([]waagent.AppointmentSummary, 0, len(resp.Items))
	for _, appt := range resp.Items {
		summary := waagent.AppointmentSummary{
			AppointmentID: appt.ID.String(),
			LeadID:        uuidPtrToString(appt.LeadID),
			LeadServiceID: uuidPtrToString(appt.LeadServiceID),
			AssignedUserID: appt.UserID.String(),
			Title:         appt.Title,
			StartTime:     appt.StartTime.Format(time.RFC3339),
			EndTime:       appt.EndTime.Format(time.RFC3339),
			Status:        string(appt.Status),
		}
		if appt.Description != nil {
			summary.Description = *appt.Description
		}
		if appt.Location != nil {
			summary.Location = *appt.Location
		}
		out = append(out, summary)
	}
	return out, nil
}

func uuidPtrToString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}
