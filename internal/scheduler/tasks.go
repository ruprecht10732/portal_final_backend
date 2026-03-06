package scheduler

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TaskAppointmentReminder = "appointments.reminder"

const TaskNotificationOutboxDue = "notification.outbox.due"

const TaskGenerateQuoteJob = "quotes.generate"
const TaskGenerateAcceptedQuotePDF = "quotes.generate_accepted_pdf"
const TaskLogCall = "leads.log_call"
const TaskIMAPSyncAccount = "imap.sync.account"
const TaskIMAPSyncSweep = "imap.sync.sweep"
const TaskApplyHumanFeedbackMemory = "leads.human_feedback.apply_memory"

type AppointmentReminderPayload struct {
	AppointmentID  string `json:"appointmentId"`
	OrganizationID string `json:"organizationId"`
}

type NotificationOutboxDuePayload struct {
	OutboxID string `json:"outboxId"`
	TenantID string `json:"tenantId"`
}

type GenerateQuoteJobPayload struct {
	JobID         string  `json:"jobId"`
	TenantID      string  `json:"tenantId"`
	UserID        string  `json:"userId"`
	LeadID        string  `json:"leadId"`
	LeadServiceID string  `json:"leadServiceId"`
	Prompt        string  `json:"prompt"`
	QuoteID       *string `json:"quoteId,omitempty"`
	Force         bool    `json:"force,omitempty"`
}

type GenerateAcceptedQuotePDFPayload struct {
	QuoteID       string `json:"quoteId"`
	TenantID      string `json:"tenantId"`
	OrgName       string `json:"orgName"`
	CustomerName  string `json:"customerName"`
	SignatureName string `json:"signatureName"`
}

type LogCallPayload struct {
	TenantID  string `json:"tenantId"`
	LeadID    string `json:"leadId"`
	ServiceID string `json:"serviceId"`
	UserID    string `json:"userId"`
	Summary   string `json:"summary"`
}

type IMAPSyncAccountPayload struct {
	AccountID string `json:"accountId"`
	UserID    string `json:"userId"`
}

type IMAPSyncSweepPayload struct{}

type ApplyHumanFeedbackMemoryPayload struct {
	TenantID   string `json:"tenantId"`
	FeedbackID string `json:"feedbackId"`
}

func NewAppointmentReminderTask(payload AppointmentReminderPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAppointmentReminder, data), nil
}

func ParseAppointmentReminderPayload(task *asynq.Task) (AppointmentReminderPayload, error) {
	var payload AppointmentReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AppointmentReminderPayload{}, err
	}
	return payload, nil
}

func NewNotificationOutboxDueTask(payload NotificationOutboxDuePayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskNotificationOutboxDue, data), nil
}

func ParseNotificationOutboxDuePayload(task *asynq.Task) (NotificationOutboxDuePayload, error) {
	var payload NotificationOutboxDuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return NotificationOutboxDuePayload{}, err
	}
	return payload, nil
}

func NewGenerateQuoteJobTask(payload GenerateQuoteJobPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGenerateQuoteJob, data), nil
}

func ParseGenerateQuoteJobPayload(task *asynq.Task) (GenerateQuoteJobPayload, error) {
	var payload GenerateQuoteJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return GenerateQuoteJobPayload{}, err
	}
	return payload, nil
}

func NewGenerateAcceptedQuotePDFTask(payload GenerateAcceptedQuotePDFPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGenerateAcceptedQuotePDF, data), nil
}

func ParseGenerateAcceptedQuotePDFPayload(task *asynq.Task) (GenerateAcceptedQuotePDFPayload, error) {
	var payload GenerateAcceptedQuotePDFPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return GenerateAcceptedQuotePDFPayload{}, err
	}
	return payload, nil
}

func NewLogCallTask(payload LogCallPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskLogCall, data), nil
}

func ParseLogCallPayload(task *asynq.Task) (LogCallPayload, error) {
	var payload LogCallPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return LogCallPayload{}, err
	}
	return payload, nil
}

func NewIMAPSyncAccountTask(payload IMAPSyncAccountPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskIMAPSyncAccount, data), nil
}

func ParseIMAPSyncAccountPayload(task *asynq.Task) (IMAPSyncAccountPayload, error) {
	var payload IMAPSyncAccountPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return IMAPSyncAccountPayload{}, err
	}
	return payload, nil
}

func NewIMAPSyncSweepTask(payload IMAPSyncSweepPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskIMAPSyncSweep, data), nil
}

func NewApplyHumanFeedbackMemoryTask(payload ApplyHumanFeedbackMemoryPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskApplyHumanFeedbackMemory, data), nil
}

func ParseApplyHumanFeedbackMemoryPayload(task *asynq.Task) (ApplyHumanFeedbackMemoryPayload, error) {
	var payload ApplyHumanFeedbackMemoryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return ApplyHumanFeedbackMemoryPayload{}, err
	}
	return payload, nil
}
