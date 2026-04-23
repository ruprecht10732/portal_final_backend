package adapters

import (
	"context"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// AppointmentsAdapter adapts the appointments service for the leads domain.
type AppointmentsAdapter struct {
	apptService *service.Service
}

func NewAppointmentsAdapter(apptService *service.Service) *AppointmentsAdapter {
	return &AppointmentsAdapter{apptService: apptService}
}

func (a *AppointmentsAdapter) BookLeadVisit(ctx context.Context, params ports.BookVisitParams) error {
	sendEmail := params.SendConfirmationEmail
	req := transport.CreateAppointmentRequest{
		LeadID:                &params.LeadID,
		LeadServiceID:         &params.LeadServiceID,
		Type:                  transport.AppointmentTypeLeadVisit,
		Title:                 params.Title,
		Description:           params.Description,
		StartTime:             params.StartTime,
		EndTime:               params.EndTime,
		AllDay:                false,
		SendConfirmationEmail: &sendEmail,
	}

	_, err := a.apptService.Create(ctx, params.UserID, false, params.TenantID, req)
	return err
}

func (a *AppointmentsAdapter) GetLeadVisitByService(ctx context.Context, tenantID, leadServiceID, userID uuid.UUID) (*ports.LeadVisitSummary, error) {
	appt, err := a.getAppt(ctx, tenantID, leadServiceID, userID)
	if err != nil {
		return nil, err
	}

	return &ports.LeadVisitSummary{
		AppointmentID: appt.ID,
		UserID:        appt.UserID,
		StartTime:     appt.StartTime,
		EndTime:       appt.EndTime,
	}, nil
}

func (a *AppointmentsAdapter) RescheduleLeadVisit(ctx context.Context, params ports.RescheduleVisitParams) error {
	appt, err := a.getAppt(ctx, params.TenantID, params.LeadServiceID, params.UserID)
	if err != nil {
		return err
	}

	// Fixed: Captured the *AppointmentResponse return value to resolve "WrongResultCount"
	_, err = a.apptService.Update(ctx, appt.ID, params.UserID, false, params.TenantID, transport.UpdateAppointmentRequest{
		Title:       params.Title,
		Description: params.Description,
		StartTime:   &params.StartTime,
		EndTime:     &params.EndTime,
	})
	return err
}

func (a *AppointmentsAdapter) CancelLeadVisit(ctx context.Context, params ports.CancelVisitParams) error {
	appt, err := a.getAppt(ctx, params.TenantID, params.LeadServiceID, params.UserID)
	if err != nil {
		return err
	}

	req := transport.UpdateAppointmentStatusRequest{Status: transport.AppointmentStatusCancelled}
	_, err = a.apptService.UpdateStatus(ctx, appt.ID, params.UserID, false, params.TenantID, req)
	return err
}

func (a *AppointmentsAdapter) getAppt(ctx context.Context, tenantID, leadServiceID, userID uuid.UUID) (*transport.AppointmentResponse, error) {
	return a.apptService.GetByLeadServiceID(ctx, leadServiceID, userID, false, tenantID)
}

var _ ports.AppointmentBooker = (*AppointmentsAdapter)(nil)
