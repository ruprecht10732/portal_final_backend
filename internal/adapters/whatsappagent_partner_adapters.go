package adapters

import (
	"context"
	"fmt"
	"strings"

	appointmentservice "portal_final_backend/internal/appointments/service"
	appointmenttransport "portal_final_backend/internal/appointments/transport"
	partnersservice "portal_final_backend/internal/partners/service"
	partnerstransport "portal_final_backend/internal/partners/transport"
	whatsappagent "portal_final_backend/internal/whatsappagent"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	errAppointmentsServiceNotConfigured = "appointments service not configured"
	timeRFC3339OffsetLayout             = "2006-01-02T15:04:05Z07:00"
)

type WhatsAppAgentPartnerAdapter struct {
	partners     *partnersservice.Service
	appointments *appointmentservice.Service
}

func NewWhatsAppAgentPartnerAdapter(partners *partnersservice.Service, appointments *appointmentservice.Service) *WhatsAppAgentPartnerAdapter {
	return &WhatsAppAgentPartnerAdapter{partners: partners, appointments: appointments}
}

func (a *WhatsAppAgentPartnerAdapter) GetPartnerPhone(ctx context.Context, orgID, partnerID uuid.UUID) (*whatsappagent.PartnerPhoneRecord, error) {
	if a == nil || a.partners == nil {
		return nil, fmt.Errorf("partner service not configured")
	}
	partner, err := a.partners.GetByID(ctx, orgID, partnerID)
	if err != nil {
		return nil, err
	}
	phone := strings.TrimSpace(partner.ContactPhone)
	if phone == "" {
		return nil, apperr.NotFound("partner has no contact phone")
	}
	displayName := strings.TrimSpace(partner.ContactName)
	if displayName == "" {
		displayName = strings.TrimSpace(partner.BusinessName)
	}
	return &whatsappagent.PartnerPhoneRecord{
		PartnerID:    partner.ID,
		DisplayName:  displayName,
		PhoneNumber:  phone,
		BusinessName: partner.BusinessName,
	}, nil
}

func (a *WhatsAppAgentPartnerAdapter) ListPartnerJobs(ctx context.Context, orgID, partnerID uuid.UUID) ([]whatsappagent.PartnerJobSummary, error) {
	offers, err := a.listAcceptedOffers(ctx, orgID, partnerID, "")
	if err != nil {
		return nil, err
	}
	jobs := make([]whatsappagent.PartnerJobSummary, 0, len(offers.Items))
	for _, offer := range offers.Items {
		job := a.buildPartnerJobSummary(ctx, orgID, partnerID, offer)
		if !isActivePartnerJob(job) {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (a *WhatsAppAgentPartnerAdapter) GetPartnerJobByLead(ctx context.Context, orgID, partnerID, leadID uuid.UUID) (*whatsappagent.PartnerJobSummary, error) {
	offers, err := a.listAcceptedOffers(ctx, orgID, partnerID, "")
	if err != nil {
		return nil, err
	}
	for _, offer := range offers.Items {
		job := a.buildPartnerJobSummary(ctx, orgID, partnerID, offer)
		if job.LeadID == leadID.String() {
			if !isActivePartnerJob(job) {
				return nil, apperr.Forbidden("deze opdracht is niet meer actief")
			}
			return &job, nil
		}
	}
	return nil, apperr.Forbidden("deze opdracht hoort niet bij deze partner")
}

func (a *WhatsAppAgentPartnerAdapter) GetPartnerJobByService(ctx context.Context, orgID, partnerID, leadServiceID uuid.UUID) (*whatsappagent.PartnerJobSummary, error) {
	offers, err := a.listAcceptedOffers(ctx, orgID, partnerID, leadServiceID.String())
	if err != nil {
		return nil, err
	}
	if len(offers.Items) == 0 {
		return nil, apperr.Forbidden("deze opdracht hoort niet bij deze partner")
	}
	job := a.buildPartnerJobSummary(ctx, orgID, partnerID, offers.Items[0])
	if !isActivePartnerJob(job) {
		return nil, apperr.Forbidden("deze opdracht is niet meer actief")
	}
	return &job, nil
}

func (a *WhatsAppAgentPartnerAdapter) GetPartnerJobByAppointment(ctx context.Context, orgID, partnerID, appointmentID uuid.UUID) (*whatsappagent.PartnerJobSummary, error) {
	if a == nil || a.appointments == nil {
		return nil, fmt.Errorf(errAppointmentsServiceNotConfigured)
	}
	appointment, err := a.appointments.GetByID(ctx, appointmentID, uuid.Nil, true, orgID)
	if err != nil {
		return nil, err
	}
	if appointment.LeadServiceID == nil {
		return nil, apperr.Forbidden("deze afspraak hoort niet bij een partneropdracht")
	}
	job, err := a.GetPartnerJobByService(ctx, orgID, partnerID, *appointment.LeadServiceID)
	if err != nil {
		return nil, err
	}
	job.AppointmentID = appointment.ID.String()
	job.AppointmentTitle = appointment.Title
	job.AppointmentStatus = string(appointment.Status)
	job.AppointmentStart = appointment.StartTime.Format(timeRFC3339OffsetLayout)
	job.AppointmentEnd = appointment.EndTime.Format(timeRFC3339OffsetLayout)
	if appointment.Lead != nil {
		job.CustomerName = strings.TrimSpace(strings.TrimSpace(appointment.Lead.FirstName) + " " + strings.TrimSpace(appointment.Lead.LastName))
		job.DestinationAddress = appointment.Lead.Address
		if job.City == "" {
			job.City = appointment.Lead.Address
		}
		job.LeadID = appointment.Lead.ID.String()
	}
	return job, nil
}

func (a *WhatsAppAgentPartnerAdapter) UpsertVisitReport(ctx context.Context, orgID, appointmentID uuid.UUID, input whatsappagent.SaveMeasurementInput) error {
	if a == nil || a.appointments == nil {
		return fmt.Errorf(errAppointmentsServiceNotConfigured)
	}
	request := appointmenttransport.UpsertVisitReportRequest{}
	if value := strings.TrimSpace(input.Measurements); value != "" {
		request.Measurements = &value
	}
	if value := normalizeAccessDifficulty(input.AccessDifficulty); value != nil {
		request.AccessDifficulty = value
	}
	if value := strings.TrimSpace(input.Notes); value != "" {
		request.Notes = &value
	}
	_, err := a.appointments.UpsertVisitReport(ctx, appointmentID, uuid.Nil, true, orgID, request)
	return err
}

func (a *WhatsAppAgentPartnerAdapter) UpdateAppointmentStatus(ctx context.Context, orgID, appointmentID uuid.UUID, input whatsappagent.UpdateAppointmentStatusInput) (*whatsappagent.AppointmentSummary, error) {
	if a == nil || a.appointments == nil {
		return nil, fmt.Errorf(errAppointmentsServiceNotConfigured)
	}
	updated, err := a.appointments.UpdateStatus(ctx, appointmentID, uuid.Nil, true, orgID, appointmenttransport.UpdateAppointmentStatusRequest{
		Status: appointmenttransport.AppointmentStatus(input.Status),
	})
	if err != nil {
		return nil, err
	}
	summary := &whatsappagent.AppointmentSummary{
		AppointmentID:  updated.ID.String(),
		AssignedUserID: updated.UserID.String(),
		Title:          updated.Title,
		StartTime:      updated.StartTime.Format(timeRFC3339OffsetLayout),
		EndTime:        updated.EndTime.Format(timeRFC3339OffsetLayout),
		Status:         string(updated.Status),
	}
	if updated.LeadID != nil {
		summary.LeadID = updated.LeadID.String()
	}
	if updated.LeadServiceID != nil {
		summary.LeadServiceID = updated.LeadServiceID.String()
	}
	if updated.Description != nil {
		summary.Description = *updated.Description
	}
	if updated.Location != nil {
		summary.Location = *updated.Location
	}
	return summary, nil
}

func (a *WhatsAppAgentPartnerAdapter) listAcceptedOffers(ctx context.Context, orgID, partnerID uuid.UUID, leadServiceID string) (partnerstransport.OfferListResponse, error) {
	if a == nil || a.partners == nil {
		return partnerstransport.OfferListResponse{}, fmt.Errorf("partner service not configured")
	}
	return a.partners.ListOffers(ctx, orgID, partnerstransport.ListOffersRequest{
		PartnerID:     partnerID.String(),
		LeadServiceID: strings.TrimSpace(leadServiceID),
		Status:        "accepted",
		Page:          1,
		PageSize:      100,
		SortBy:        "createdAt",
		SortOrder:     "desc",
	})
}

func (a *WhatsAppAgentPartnerAdapter) buildPartnerJobSummary(ctx context.Context, orgID, partnerID uuid.UUID, offer partnerstransport.OfferResponse) whatsappagent.PartnerJobSummary {
	job := whatsappagent.PartnerJobSummary{
		OfferID:          offer.ID.String(),
		PartnerID:        partnerID.String(),
		LeadServiceID:    offer.LeadServiceID.String(),
		ServiceType:      valueOrDefault(offer.ServiceType),
		City:             valueOrDefault(offer.LeadCity),
		VakmanPriceCents: offer.VakmanPriceCents,
		OfferStatus:      offer.Status,
	}
	if a == nil || a.appointments == nil {
		return job
	}
	appointment, err := a.appointments.GetByLeadServiceID(ctx, offer.LeadServiceID, uuid.Nil, true, orgID)
	if err != nil || appointment == nil {
		return job
	}
	job.AppointmentID = appointment.ID.String()
	job.AppointmentTitle = appointment.Title
	job.AppointmentStatus = string(appointment.Status)
	job.AppointmentStart = appointment.StartTime.Format(timeRFC3339OffsetLayout)
	job.AppointmentEnd = appointment.EndTime.Format(timeRFC3339OffsetLayout)
	if appointment.LeadID != nil {
		job.LeadID = appointment.LeadID.String()
	}
	if appointment.Lead != nil {
		job.CustomerName = strings.TrimSpace(strings.TrimSpace(appointment.Lead.FirstName) + " " + strings.TrimSpace(appointment.Lead.LastName))
		job.DestinationAddress = appointment.Lead.Address
		if job.City == "" {
			job.City = appointment.Lead.Address
		}
	}
	return job
}

func normalizeAccessDifficulty(raw string) *appointmenttransport.AccessDifficulty {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "laag":
		value := appointmenttransport.AccessDifficultyLow
		return &value
	case "medium", "middel", "gemiddeld":
		value := appointmenttransport.AccessDifficultyMedium
		return &value
	case "high", "hoog":
		value := appointmenttransport.AccessDifficultyHigh
		return &value
	default:
		return nil
	}
}

func valueOrDefault[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func isActivePartnerJob(job whatsappagent.PartnerJobSummary) bool {
	status := strings.ToLower(strings.TrimSpace(job.AppointmentStatus))
	switch status {
	case "completed", "cancelled", "no_show":
		return false
	default:
		return true
	}
}
