package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	apptsvc "portal_final_backend/internal/appointments/service"
	appttransport "portal_final_backend/internal/appointments/transport"
	leadsmgmt "portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/ports"
	leadsrepo "portal_final_backend/internal/leads/repository"
	leadtransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/waagent"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

const waagentActorName = "Reinout"

const errInvalidLeadID = "invalid lead_id"

type WAAgentLeadActionsAdapter struct {
	mgmt *leadsmgmt.Service
	repo leadsrepo.LeadsRepository
}

func NewWAAgentLeadActionsAdapter(mgmt *leadsmgmt.Service, repo leadsrepo.LeadsRepository) *WAAgentLeadActionsAdapter {
	return &WAAgentLeadActionsAdapter{mgmt: mgmt, repo: repo}
}

func (a *WAAgentLeadActionsAdapter) SearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) ([]waagent.LeadSearchResult, error) {
	if a.mgmt == nil {
		return nil, fmt.Errorf("lead management service not configured")
	}
	if limit <= 0 {
		limit = 5
	}
	resp, err := a.mgmt.List(ctx, leadtransport.ListLeadsRequest{Search: strings.TrimSpace(query), Page: 1, PageSize: limit}, orgID)
	if err != nil {
		return nil, err
	}
	results := make([]waagent.LeadSearchResult, 0, len(resp.Items))
	for _, item := range resp.Items {
		serviceID := ""
		serviceType := ""
		status := ""
		if item.CurrentService != nil {
			serviceID = item.CurrentService.ID.String()
			serviceType = string(item.CurrentService.ServiceType)
			status = string(item.CurrentService.Status)
		}
		results = append(results, waagent.LeadSearchResult{
			LeadID:        item.ID.String(),
			LeadServiceID: serviceID,
			CustomerName:  strings.TrimSpace(item.Consumer.FirstName + " " + item.Consumer.LastName),
			Phone:         item.Consumer.Phone,
			City:          item.Address.City,
			ServiceType:   serviceType,
			Status:        status,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
		})
	}
	return results, nil
}

func (a *WAAgentLeadActionsAdapter) UpdateLeadDetails(ctx context.Context, orgID uuid.UUID, input waagent.UpdateLeadDetailsInput) ([]string, error) {
	if a.mgmt == nil {
		return nil, fmt.Errorf("lead management service not configured")
	}
	leadID, err := uuid.Parse(strings.TrimSpace(input.LeadID))
	if err != nil {
		return nil, fmt.Errorf(errInvalidLeadID)
	}
	req := leadtransport.UpdateLeadRequest{
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Email:           input.Email,
		Street:          input.Street,
		HouseNumber:     input.HouseNumber,
		ZipCode:         input.ZipCode,
		City:            input.City,
		Latitude:        input.Latitude,
		Longitude:       input.Longitude,
		WhatsAppOptedIn: input.WhatsAppOptedIn,
	}
	if input.Phone != nil {
		normalized := strings.TrimSpace(phone.NormalizeE164(*input.Phone))
		req.Phone = &normalized
	}
	if input.ConsumerRole != nil {
		role := leadtransport.ConsumerRole(strings.TrimSpace(*input.ConsumerRole))
		req.ConsumerRole = &role
	}
	if _, err := a.mgmt.Update(ctx, leadID, req, uuid.Nil, orgID, nil); err != nil {
		return nil, err
	}
	updatedFields := make([]string, 0, 10)
	appendField := func(name string, present bool) {
		if present {
			updatedFields = append(updatedFields, name)
		}
	}
	appendField("firstName", input.FirstName != nil)
	appendField("lastName", input.LastName != nil)
	appendField("phone", input.Phone != nil)
	appendField("email", input.Email != nil)
	appendField("consumerRole", input.ConsumerRole != nil)
	appendField("street", input.Street != nil)
	appendField("houseNumber", input.HouseNumber != nil)
	appendField("zipCode", input.ZipCode != nil)
	appendField("city", input.City != nil)
	appendField("latitude", input.Latitude != nil)
	appendField("longitude", input.Longitude != nil)
	appendField("whatsAppOptedIn", input.WhatsAppOptedIn != nil)
	return updatedFields, nil
}

func (a *WAAgentLeadActionsAdapter) AskCustomerClarification(ctx context.Context, orgID uuid.UUID, input waagent.AskCustomerClarificationInput) error {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return err
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return fmt.Errorf("message is required")
	}
	_, err = a.repo.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: orgID,
		ActorType:      leadsrepo.ActorTypeAI,
		ActorName:      waagentActorName,
		EventType:      leadsrepo.EventTypeNote,
		Title:          leadsrepo.EventTitleNoteAdded,
		Summary:        &message,
		Metadata: map[string]any{
			"noteType":          "ai_clarification_request",
			"missingDimensions": input.MissingDimensions,
			"source":            "whatsapp_agent",
		},
	})
	return err
}

func (a *WAAgentLeadActionsAdapter) SaveNote(ctx context.Context, orgID uuid.UUID, input waagent.SaveNoteInput) error {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return err
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return fmt.Errorf("body is required")
	}
	_, err = a.repo.CreateTimelineEvent(ctx, leadsrepo.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: orgID,
		ActorType:      leadsrepo.ActorTypeAI,
		ActorName:      waagentActorName,
		EventType:      leadsrepo.EventTypeNote,
		Title:          leadsrepo.EventTitleNoteAdded,
		Summary:        &body,
		Metadata: map[string]any{
			"noteType": "whatsapp_agent_note",
			"source":   "whatsapp_agent",
		},
	})
	return err
}

func (a *WAAgentLeadActionsAdapter) UpdateLeadStatus(ctx context.Context, orgID uuid.UUID, input waagent.UpdateStatusInput) (string, error) {
	leadID, serviceID, err := a.resolveLeadAndService(ctx, orgID, input.LeadID, input.LeadServiceID)
	if err != nil {
		return "", err
	}
	resp, err := a.mgmt.UpdateServiceStatus(ctx, leadID, serviceID, leadtransport.UpdateServiceStatusRequest{Status: leadtransport.LeadStatus(input.Status)}, orgID)
	if err != nil {
		return "", err
	}
	if resp.CurrentService != nil {
		return string(resp.CurrentService.Status), nil
	}
	return input.Status, nil
}

func (a *WAAgentLeadActionsAdapter) resolveLeadAndService(ctx context.Context, orgID uuid.UUID, leadIDRaw string, serviceIDRaw string) (uuid.UUID, uuid.UUID, error) {
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf(errInvalidLeadID)
	}
	if trimmed := strings.TrimSpace(serviceIDRaw); trimmed != "" {
		serviceID, parseErr := uuid.Parse(trimmed)
		if parseErr != nil {
			return uuid.Nil, uuid.Nil, fmt.Errorf("invalid lead_service_id")
		}
		return leadID, serviceID, nil
	}
	svc, err := a.repo.GetCurrentLeadService(ctx, leadID, orgID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return leadID, svc.ID, nil
}

type WAAgentVisitActionsAdapter struct {
	slots ports.AppointmentSlotProvider
	svc   *apptsvc.Service
	repo  leadsrepo.LeadsRepository
}

func NewWAAgentVisitActionsAdapter(slots ports.AppointmentSlotProvider, svc *apptsvc.Service, repo leadsrepo.LeadsRepository) *WAAgentVisitActionsAdapter {
	return &WAAgentVisitActionsAdapter{slots: slots, svc: svc, repo: repo}
}

func (a *WAAgentVisitActionsAdapter) GetAvailableVisitSlots(ctx context.Context, orgID uuid.UUID, startDate, endDate string, slotDuration int) ([]waagent.VisitSlotSummary, error) {
	if a.slots == nil {
		return nil, fmt.Errorf("appointment slot provider not configured")
	}
	resp, err := a.slots.GetAvailableSlots(ctx, orgID, startDate, endDate, slotDuration)
	if err != nil {
		return nil, err
	}
	slots := make([]waagent.VisitSlotSummary, 0)
	for _, day := range resp.Days {
		for _, slot := range day.Slots {
			slots = append(slots, waagent.VisitSlotSummary{
				AssignedUserID: slot.UserID.String(),
				StartTime:      slot.StartTime.Format(time.RFC3339),
				EndTime:        slot.EndTime.Format(time.RFC3339),
				Date:           day.Date,
			})
		}
	}
	return slots, nil
}

func (a *WAAgentVisitActionsAdapter) ScheduleVisit(ctx context.Context, orgID uuid.UUID, input waagent.ScheduleVisitInput) (*waagent.AppointmentSummary, error) {
	if a.slots == nil {
		return nil, fmt.Errorf("appointment slot provider not configured")
	}
	leadID, err := uuid.Parse(strings.TrimSpace(input.LeadID))
	if err != nil {
		return nil, fmt.Errorf(errInvalidLeadID)
	}
	serviceID, err := uuid.Parse(strings.TrimSpace(input.LeadServiceID))
	if err != nil {
		return nil, fmt.Errorf("invalid lead_service_id")
	}
	userID, err := uuid.Parse(strings.TrimSpace(input.AssignedUserID))
	if err != nil {
		return nil, fmt.Errorf("invalid assigned_user_id")
	}
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartTime))
	if err != nil {
		return nil, fmt.Errorf("invalid start_time")
	}
	endTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndTime))
	if err != nil {
		return nil, fmt.Errorf("invalid end_time")
	}
	appointment, err := a.slots.CreateRequestedAppointment(ctx, userID, orgID, leadID, serviceID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return &waagent.AppointmentSummary{
		AppointmentID: appointment.ID.String(),
		LeadID:        leadID.String(),
		LeadServiceID: serviceID.String(),
		AssignedUserID: userID.String(),
		Title:         appointment.Title,
		Description:   derefString(appointment.Description),
		Location:      derefString(appointment.Location),
		StartTime:     appointment.StartTime.Format(time.RFC3339),
		EndTime:       appointment.EndTime.Format(time.RFC3339),
		Status:        appointment.Status,
	}, nil
}

func (a *WAAgentVisitActionsAdapter) RescheduleVisit(ctx context.Context, orgID uuid.UUID, input waagent.RescheduleVisitInput) (*waagent.AppointmentSummary, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("appointment service not configured")
	}
	appointmentID, err := uuid.Parse(strings.TrimSpace(input.AppointmentID))
	if err != nil {
		return nil, fmt.Errorf("invalid appointment_id")
	}
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartTime))
	if err != nil {
		return nil, fmt.Errorf("invalid start_time")
	}
	endTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndTime))
	if err != nil {
		return nil, fmt.Errorf("invalid end_time")
	}
	resp, err := a.svc.Update(ctx, appointmentID, uuid.Nil, true, orgID, appttransport.UpdateAppointmentRequest{
		Title:       input.Title,
		Description: input.Description,
		StartTime:   &startTime,
		EndTime:     &endTime,
	})
	if err != nil {
		return nil, err
	}
	return appointmentSummaryFromResponse(resp), nil
}

func (a *WAAgentVisitActionsAdapter) CancelVisit(ctx context.Context, orgID uuid.UUID, input waagent.CancelVisitInput) error {
	if a.svc == nil {
		return fmt.Errorf("appointment service not configured")
	}
	appointmentID, err := uuid.Parse(strings.TrimSpace(input.AppointmentID))
	if err != nil {
		return fmt.Errorf("invalid appointment_id")
	}
	resp, err := a.svc.UpdateStatus(ctx, appointmentID, uuid.Nil, true, orgID, appttransport.UpdateAppointmentStatusRequest{Status: appttransport.AppointmentStatusCancelled})
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.Reason) != "" && resp.LeadID != nil {
		serviceID := ""
		if resp.LeadServiceID != nil {
			serviceID = resp.LeadServiceID.String()
		}
		return (&WAAgentLeadActionsAdapter{repo: a.repo}).SaveNote(ctx, orgID, waagent.SaveNoteInput{
			LeadID:        resp.LeadID.String(),
			LeadServiceID: serviceID,
			Body:          strings.TrimSpace(input.Reason),
		})
	}
	return nil
}

func appointmentSummaryFromResponse(resp *appttransport.AppointmentResponse) *waagent.AppointmentSummary {
	if resp == nil {
		return nil
	}
	leadID := ""
	if resp.LeadID != nil {
		leadID = resp.LeadID.String()
	}
	serviceID := ""
	if resp.LeadServiceID != nil {
		serviceID = resp.LeadServiceID.String()
	}
	return &waagent.AppointmentSummary{
		AppointmentID: resp.ID.String(),
		LeadID:        leadID,
		LeadServiceID: serviceID,
		AssignedUserID: resp.UserID.String(),
		Title:         resp.Title,
		Description:   derefString(resp.Description),
		Location:      derefString(resp.Location),
		StartTime:     resp.StartTime.Format(time.RFC3339),
		EndTime:       resp.EndTime.Format(time.RFC3339),
		Status:        string(resp.Status),
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}