package adapters

import (
	"context"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/leads/ports"
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

// Compile-time check that AppointmentsAdapter implements ports.AppointmentBooker
var _ ports.AppointmentBooker = (*AppointmentsAdapter)(nil)
