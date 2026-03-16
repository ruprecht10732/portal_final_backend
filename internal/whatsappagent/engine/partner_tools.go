package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

const (
	errPartnerJobReaderNotConfigured        = "partner job reader not configured"
	errPartnerContextUnavailable            = "partner context unavailable"
	errAppointmentStatusWriterNotConfigured = "appointment status writer not configured"
	errVisitReportWriterNotConfigured       = "appointment visit report writer not configured"
	errInvalidAppointmentID                 = "appointment_id is ongeldig"
)

type GetMyJobsInput struct {
	Limit int `json:"limit,omitempty"`
}

type GetMyJobsOutput struct {
	Jobs  []PartnerJobSummary `json:"jobs"`
	Count int                 `json:"count"`
}

type SaveMeasurementInput struct {
	AppointmentID    string `json:"appointment_id"`
	Measurements     string `json:"measurements,omitempty"`
	AccessDifficulty string `json:"access_difficulty,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

type SaveMeasurementOutput struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	AppointmentID string `json:"appointment_id,omitempty"`
}

type GetPartnerJobDetailsInput struct {
	AppointmentID string `json:"appointment_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	LeadID        string `json:"lead_id,omitempty"`
}

type GetPartnerJobDetailsOutput struct {
	Success bool               `json:"success"`
	Message string             `json:"message"`
	Job     *PartnerJobSummary `json:"job,omitempty"`
}

type UpdateAppointmentStatusInput struct {
	AppointmentID string `json:"appointment_id"`
	Status        string `json:"status"`
}

type UpdateAppointmentStatusOutput struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Appointment *AppointmentSummary `json:"appointment,omitempty"`
}

func (h *ToolHandler) HandleGetMyJobs(ctx tool.Context, orgID uuid.UUID, input GetMyJobsInput) (GetMyJobsOutput, error) {
	if h.partnerJobReader == nil {
		return GetMyJobsOutput{}, fmt.Errorf(errPartnerJobReaderNotConfigured)
	}
	partnerID, ok := partnerIDFromToolContext(ctx)
	if !ok {
		return GetMyJobsOutput{}, fmt.Errorf(errPartnerContextUnavailable)
	}
	jobs, err := h.partnerJobReader.ListPartnerJobs(context.Background(), orgID, partnerID)
	if err != nil {
		return GetMyJobsOutput{}, err
	}
	if input.Limit > 0 && len(jobs) > input.Limit {
		jobs = jobs[:input.Limit]
	}
	return GetMyJobsOutput{Jobs: jobs, Count: len(jobs)}, nil
}

func (h *ToolHandler) HandleSaveMeasurement(ctx tool.Context, orgID uuid.UUID, input SaveMeasurementInput) (SaveMeasurementOutput, error) {
	if h.partnerJobReader == nil {
		return SaveMeasurementOutput{}, fmt.Errorf(errPartnerJobReaderNotConfigured)
	}
	if h.appointmentVisitReportWriter == nil {
		return SaveMeasurementOutput{}, fmt.Errorf(errVisitReportWriterNotConfigured)
	}
	partnerID, ok := partnerIDFromToolContext(ctx)
	if !ok {
		return SaveMeasurementOutput{}, fmt.Errorf(errPartnerContextUnavailable)
	}
	appointmentIDText := strings.TrimSpace(input.AppointmentID)
	if appointmentIDText == "" {
		return SaveMeasurementOutput{Success: false, Message: "appointment_id is verplicht"}, fmt.Errorf("appointment_id is required")
	}
	appointmentID, err := uuid.Parse(appointmentIDText)
	if err != nil {
		return SaveMeasurementOutput{Success: false, Message: errInvalidAppointmentID}, err
	}
	if strings.TrimSpace(input.Measurements) == "" && strings.TrimSpace(input.AccessDifficulty) == "" && strings.TrimSpace(input.Notes) == "" {
		return SaveMeasurementOutput{Success: false, Message: "Voeg minimaal metingen, toegankelijkheid of notities toe"}, fmt.Errorf("empty measurement payload")
	}
	if _, err := h.partnerJobReader.GetPartnerJobByAppointment(context.Background(), orgID, partnerID, appointmentID); err != nil {
		return SaveMeasurementOutput{Success: false, Message: err.Error()}, err
	}
	if err := h.appointmentVisitReportWriter.UpsertVisitReport(context.Background(), orgID, appointmentID, input); err != nil {
		return SaveMeasurementOutput{Success: false, Message: err.Error()}, err
	}
	return SaveMeasurementOutput{Success: true, Message: "Metingen opgeslagen", AppointmentID: appointmentID.String()}, nil
}

func (h *ToolHandler) HandleGetPartnerJobDetails(ctx tool.Context, orgID uuid.UUID, input GetPartnerJobDetailsInput) (GetPartnerJobDetailsOutput, error) {
	if h.partnerJobReader == nil {
		return GetPartnerJobDetailsOutput{}, fmt.Errorf(errPartnerJobReaderNotConfigured)
	}
	partnerID, ok := partnerIDFromToolContext(ctx)
	if !ok {
		return GetPartnerJobDetailsOutput{}, fmt.Errorf(errPartnerContextUnavailable)
	}
	job, err := h.resolvePartnerJob(ctx, orgID, partnerID, input.AppointmentID, input.LeadServiceID, input.LeadID)
	if err != nil {
		return GetPartnerJobDetailsOutput{Success: false, Message: err.Error()}, err
	}
	return GetPartnerJobDetailsOutput{Success: true, Message: "Opdracht gevonden", Job: job}, nil
}

func (h *ToolHandler) HandleUpdateAppointmentStatus(ctx tool.Context, orgID uuid.UUID, input UpdateAppointmentStatusInput) (UpdateAppointmentStatusOutput, error) {
	if h.partnerJobReader == nil {
		return UpdateAppointmentStatusOutput{}, fmt.Errorf(errPartnerJobReaderNotConfigured)
	}
	if h.appointmentStatusWriter == nil {
		return UpdateAppointmentStatusOutput{}, fmt.Errorf(errAppointmentStatusWriterNotConfigured)
	}
	partnerID, ok := partnerIDFromToolContext(ctx)
	if !ok {
		return UpdateAppointmentStatusOutput{}, fmt.Errorf(errPartnerContextUnavailable)
	}
	appointmentIDText := strings.TrimSpace(input.AppointmentID)
	if appointmentIDText == "" {
		return UpdateAppointmentStatusOutput{Success: false, Message: "appointment_id is verplicht"}, fmt.Errorf("appointment_id is required")
	}
	appointmentID, err := uuid.Parse(appointmentIDText)
	if err != nil {
		return UpdateAppointmentStatusOutput{Success: false, Message: errInvalidAppointmentID}, err
	}
	if _, err := h.partnerJobReader.GetPartnerJobByAppointment(context.Background(), orgID, partnerID, appointmentID); err != nil {
		return UpdateAppointmentStatusOutput{Success: false, Message: err.Error()}, err
	}
	input.Status = normalizeAppointmentStatus(input.Status)
	if input.Status == "" {
		return UpdateAppointmentStatusOutput{Success: false, Message: "status is ongeldig"}, fmt.Errorf("invalid appointment status")
	}
	appointment, err := h.appointmentStatusWriter.UpdateAppointmentStatus(context.Background(), orgID, appointmentID, input)
	if err != nil {
		return UpdateAppointmentStatusOutput{Success: false, Message: err.Error()}, err
	}
	return UpdateAppointmentStatusOutput{Success: true, Message: "Afspraakstatus bijgewerkt", Appointment: appointment}, nil
}

func normalizeAppointmentStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "scheduled", "ingepland":
		return "scheduled"
	case "requested", "aangevraagd":
		return "requested"
	case "completed", "complete", "afgerond", "voltooid":
		return "completed"
	case "cancelled", "canceled", "geannuleerd", "afgezegd":
		return "cancelled"
	case "no_show", "no show", "niet verschenen":
		return "no_show"
	default:
		return ""
	}
}

func (h *ToolHandler) resolvePartnerJob(_ tool.Context, orgID, partnerID uuid.UUID, appointmentIDRaw, leadServiceIDRaw, leadIDRaw string) (*PartnerJobSummary, error) {
	if appointmentIDText := strings.TrimSpace(appointmentIDRaw); appointmentIDText != "" {
		appointmentID, err := uuid.Parse(appointmentIDText)
		if err != nil {
			return nil, fmt.Errorf(errInvalidAppointmentID)
		}
		return h.partnerJobReader.GetPartnerJobByAppointment(context.Background(), orgID, partnerID, appointmentID)
	}
	if leadServiceIDText := strings.TrimSpace(leadServiceIDRaw); leadServiceIDText != "" {
		leadServiceID, err := uuid.Parse(leadServiceIDText)
		if err != nil {
			return nil, fmt.Errorf("lead_service_id is ongeldig")
		}
		return h.partnerJobReader.GetPartnerJobByService(context.Background(), orgID, partnerID, leadServiceID)
	}
	if leadIDText := strings.TrimSpace(leadIDRaw); leadIDText != "" {
		leadID, err := uuid.Parse(leadIDText)
		if err != nil {
			return nil, fmt.Errorf("lead_id is ongeldig")
		}
		return h.partnerJobReader.GetPartnerJobByLead(context.Background(), orgID, partnerID, leadID)
	}
	return nil, fmt.Errorf("appointment_id, lead_service_id of lead_id is verplicht")
}
