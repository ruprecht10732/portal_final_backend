package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/tasks"
	whatsappagent "portal_final_backend/internal/whatsappagent"

	"github.com/google/uuid"
)

type WhatsAppAgentTaskWriterAdapter struct {
	tasks *tasks.Service
	leads leadsrepo.LeadReader
}

func NewWhatsAppAgentTaskWriterAdapter(tasksSvc *tasks.Service, leadsRepo leadsrepo.LeadReader) *WhatsAppAgentTaskWriterAdapter {
	return &WhatsAppAgentTaskWriterAdapter{tasks: tasksSvc, leads: leadsRepo}
}

func (a *WhatsAppAgentTaskWriterAdapter) CreateTask(ctx context.Context, orgID uuid.UUID, input whatsappagent.CreateTaskInput) (whatsappagent.CreateTaskOutput, error) {
	if a.tasks == nil {
		return whatsappagent.CreateTaskOutput{}, fmt.Errorf("task service not configured")
	}
	assignedUserID, missing, err := a.resolveCreateTaskInput(ctx, orgID, input)
	if err != nil {
		return whatsappagent.CreateTaskOutput{}, err
	}
	if len(missing) > 0 {
		return whatsappagent.CreateTaskOutput{Success: false, Message: "Er ontbreken nog gegevens om de taak aan te maken", MissingFields: missing}, nil
	}

	request, output, err := a.buildCreateTaskRequest(input, assignedUserID)
	if err != nil || output != nil {
		if output != nil {
			return *output, nil
		}
		return whatsappagent.CreateTaskOutput{}, err
	}

	actorID, err := uuid.Parse(assignedUserID)
	if err != nil {
		return whatsappagent.CreateTaskOutput{}, err
	}
	created, err := a.tasks.Create(ctx, orgID, actorID, request)
	if err != nil {
		return whatsappagent.CreateTaskOutput{}, err
	}
	return whatsappagent.CreateTaskOutput{
		Success:        true,
		Message:        "Taak aangemaakt",
		TaskID:         created.ID.String(),
		AssignedUserID: created.AssignedUserID.String(),
	}, nil
}

func (a *WhatsAppAgentTaskWriterAdapter) resolveCreateTaskInput(ctx context.Context, orgID uuid.UUID, input whatsappagent.CreateTaskInput) (string, []string, error) {
	missing := make([]string, 0, 2)
	if strings.TrimSpace(input.Title) == "" {
		missing = append(missing, "title")
	}
	assignedUserID, err := a.resolveAssignedUserID(ctx, orgID, input)
	if err != nil {
		return "", nil, err
	}
	if assignedUserID == "" {
		missing = append(missing, "assigned_user_id")
	}
	return assignedUserID, missing, nil
}

func (a *WhatsAppAgentTaskWriterAdapter) resolveAssignedUserID(ctx context.Context, orgID uuid.UUID, input whatsappagent.CreateTaskInput) (string, error) {
	assignedUserID := strings.TrimSpace(input.AssignedUserID)
	if assignedUserID != "" {
		return assignedUserID, nil
	}
	return a.resolveLeadAssignee(ctx, orgID, input.LeadID)
}

func (a *WhatsAppAgentTaskWriterAdapter) buildCreateTaskRequest(input whatsappagent.CreateTaskInput, assignedUserID string) (tasks.CreateTaskRequest, *whatsappagent.CreateTaskOutput, error) {
	request := tasks.CreateTaskRequest{
		AssignedUserID: assignedUserID,
		Title:          strings.TrimSpace(input.Title),
		Description:    optionalTrimmedString(input.Description),
		Priority:       strings.TrimSpace(input.Priority),
	}
	if output := applyTaskScope(&request, input); output != nil {
		return tasks.CreateTaskRequest{}, output, nil
	}
	dueAt, err := parseOptionalRFC3339(input.DueAt)
	if err != nil {
		return tasks.CreateTaskRequest{}, nil, err
	}
	request.DueAt = dueAt
	request.Reminder, err = buildReminderConfig(input)
	if err != nil {
		return tasks.CreateTaskRequest{}, nil, err
	}
	return request, nil, nil
}

func applyTaskScope(request *tasks.CreateTaskRequest, input whatsappagent.CreateTaskInput) *whatsappagent.CreateTaskOutput {
	if strings.TrimSpace(input.LeadID) == "" && strings.TrimSpace(input.LeadServiceID) == "" {
		request.ScopeType = tasks.ScopeGlobal
		return nil
	}
	request.ScopeType = tasks.ScopeLeadService
	request.LeadID = optionalTrimmedString(input.LeadID)
	request.LeadServiceID = optionalTrimmedString(input.LeadServiceID)
	if request.LeadID != nil && request.LeadServiceID != nil {
		return nil
	}
	return &whatsappagent.CreateTaskOutput{
		Success: false,
		Message: "Voor een leadtaak zijn lead_id en lead_service_id vereist",
		MissingFields: []string{"lead_id", "lead_service_id"},
	}
}

func buildReminderConfig(input whatsappagent.CreateTaskInput) (*tasks.ReminderConfig, error) {
	reminderAt, err := parseOptionalRFC3339(input.ReminderAt)
	if err != nil || reminderAt == nil {
		return nil, err
	}
	return &tasks.ReminderConfig{
		Enabled:      true,
		RunAt:        reminderAt,
		RepeatDaily:  input.RepeatDaily != nil && *input.RepeatDaily,
		SendEmail:    input.SendEmail == nil || *input.SendEmail,
		SendWhatsApp: input.SendWhatsApp != nil && *input.SendWhatsApp,
	}, nil
}

func (a *WhatsAppAgentTaskWriterAdapter) resolveLeadAssignee(ctx context.Context, orgID uuid.UUID, leadIDRaw string) (string, error) {
	if a.leads == nil || strings.TrimSpace(leadIDRaw) == "" {
		return "", nil
	}
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return "", err
	}
	lead, err := a.leads.GetByID(ctx, leadID, orgID)
	if err != nil || lead.AssignedAgentID == nil {
		return "", err
	}
	return lead.AssignedAgentID.String(), nil
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseOptionalRFC3339(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}