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
	whatsappagent "portal_final_backend/internal/whatsappagent"

	"github.com/google/uuid"
)

// WhatsAppAgentQuotesAdapter adapts the quotes service for the whatsappagent module.
type WhatsAppAgentQuotesAdapter struct {
	svc *quotesvc.Service
}

// NewWhatsAppAgentQuotesAdapter creates a quotes reader for the whatsappagent.
func NewWhatsAppAgentQuotesAdapter(svc *quotesvc.Service) *WhatsAppAgentQuotesAdapter {
	return &WhatsAppAgentQuotesAdapter{svc: svc}
}

func (a *WhatsAppAgentQuotesAdapter) ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]whatsappagent.QuoteSummary, error) {
	req := quotetransport.ListQuotesRequest{PageSize: 20}
	if status != nil {
		req.Status = *status
	}
	resp, err := a.svc.List(ctx, orgID, req)
	if err != nil {
		log.Printf("whatsappagent: ListQuotes error org=%s: %v", orgID, err)
		return nil, err
	}
	log.Printf("whatsappagent: ListQuotes org=%s status=%v items=%d", orgID, status, len(resp.Items))
	out := make([]whatsappagent.QuoteSummary, 0, len(resp.Items))
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
		out = append(out, whatsappagent.QuoteSummary{
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

// WhatsAppAgentAppointmentsAdapter adapts the appointments service for the whatsappagent module.
type WhatsAppAgentAppointmentsAdapter struct {
	svc *service.Service
}

// NewWhatsAppAgentAppointmentsAdapter creates an appointments reader for the whatsappagent.
func NewWhatsAppAgentAppointmentsAdapter(svc *service.Service) *WhatsAppAgentAppointmentsAdapter {
	return &WhatsAppAgentAppointmentsAdapter{svc: svc}
}

func (a *WhatsAppAgentAppointmentsAdapter) ListAppointmentsByOrganization(ctx context.Context, orgID uuid.UUID, from, to *time.Time) ([]whatsappagent.AppointmentSummary, error) {
	req := appointmenttransport.ListAppointmentsRequest{PageSize: 20}
	if from != nil {
		req.StartFrom = from.Format("2006-01-02")
	}
	if to != nil {
		req.StartTo = to.Format("2006-01-02")
	}
	resp, err := a.svc.List(ctx, uuid.Nil, true, orgID, req)
	if err != nil {
		log.Printf("whatsappagent: ListAppointments error org=%s: %v", orgID, err)
		return nil, err
	}
	log.Printf("whatsappagent: ListAppointments org=%s items=%d", orgID, len(resp.Items))
	out := make([]whatsappagent.AppointmentSummary, 0, len(resp.Items))
	for _, appt := range resp.Items {
		summary := whatsappagent.AppointmentSummary{
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
