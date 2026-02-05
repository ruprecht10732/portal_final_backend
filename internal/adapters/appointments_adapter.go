package adapters

import (
	"context"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// AppointmentsAdapter adapts the RAC_appointments service for use by the RAC_leads domain.
// It implements the RAC_leads/ports.AppointmentBooker interface.
type AppointmentsAdapter struct {
	apptService *service.Service
}

// NewAppointmentsAdapter creates a new adapter that wraps the RAC_appointments service.
func NewAppointmentsAdapter(apptService *service.Service) *AppointmentsAdapter {
	return &AppointmentsAdapter{apptService: apptService}
}

// BookLeadVisit creates a visit appointment for a specific lead and service.
// It translates the RAC_leads domain's BookVisitParams into the RAC_appointments domain's
// CreateAppointmentRequest and calls the RAC_appointments service.
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

	// Call the RAC_appointments service as the user performing the action.
	// We pass isAdmin=false since the agent is booking on their own behalf.
	_, err := a.apptService.Create(ctx, params.UserID, false, params.TenantID, req)
	return err
}

// GetLeadVisitByService retrieves the latest non-cancelled appointment for a lead service.
func (a *AppointmentsAdapter) GetLeadVisitByService(ctx context.Context, tenantID uuid.UUID, leadServiceID uuid.UUID, userID uuid.UUID) (*ports.LeadVisitSummary, error) {
	appt, err := a.apptService.GetByLeadServiceID(ctx, leadServiceID, userID, false, tenantID)
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

// RescheduleLeadVisit updates the time (and optional metadata) for a lead visit appointment.
func (a *AppointmentsAdapter) RescheduleLeadVisit(ctx context.Context, params ports.RescheduleVisitParams) error {
	appt, err := a.apptService.GetByLeadServiceID(ctx, params.LeadServiceID, params.UserID, false, params.TenantID)
	if err != nil {
		return err
	}

	req := transport.UpdateAppointmentRequest{
		Title:       params.Title,
		Description: params.Description,
		StartTime:   &params.StartTime,
		EndTime:     &params.EndTime,
	}

	_, err = a.apptService.Update(ctx, appt.ID, params.UserID, false, params.TenantID, req)
	return err
}

// CancelLeadVisit cancels the lead visit appointment for a lead service.
func (a *AppointmentsAdapter) CancelLeadVisit(ctx context.Context, params ports.CancelVisitParams) error {
	appt, err := a.apptService.GetByLeadServiceID(ctx, params.LeadServiceID, params.UserID, false, params.TenantID)
	if err != nil {
		return err
	}

	req := transport.UpdateAppointmentStatusRequest{Status: transport.AppointmentStatusCancelled}
	_, err = a.apptService.UpdateStatus(ctx, appt.ID, params.UserID, false, params.TenantID, req)
	return err
}

// Compile-time check that AppointmentsAdapter implements ports.AppointmentBooker
var _ ports.AppointmentBooker = (*AppointmentsAdapter)(nil)
