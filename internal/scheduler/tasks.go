package scheduler

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TaskAppointmentReminder = "appointments.reminder"
const TaskTaskReminder = "tasks.reminder_due"

const TaskNotificationOutboxDue = "notification.outbox.due"

const TaskGenerateQuoteJob = "quotes.generate"
const TaskGenerateAcceptedQuotePDF = "quotes.generate_accepted_pdf"
const TaskAnalyzeSubsidy = "quotes.analyze_subsidy"
const TaskLogCall = "leads.log_call"
const TaskGeneratePartnerOfferSummary = "partners.offer.generate_summary"
const TaskGeneratePartnerOfferPDF = "partners.offer.generate_pdf"
const TaskRunGatekeeper = "leads.gatekeeper.run"
const TaskRunEstimator = "leads.estimator.run"
const TaskRunDispatcher = "leads.dispatcher.run"
const TaskAuditVisitReport = "leads.audit.visit_report"
const TaskAuditCallLog = "leads.audit.call_log"
const TaskWAAgentVoiceTranscription = "whatsappagent.voice_transcription.run"
const TaskIMAPSyncAccount = "imap.sync.account"
const TaskIMAPSyncSweep = "imap.sync.sweep"
const TaskApplyHumanFeedbackMemory = "leads.human_feedback.apply_memory"
const TaskStaleLeadNotify = "leads.stale.notify"
const TaskStaleLeadReEngage = "leads.stale.reengage"
const TaskAgentRun = "agent:run"

// AgentTaskPayload is the unified payload for all agent runs.
type AgentTaskPayload struct {
	Workspace     string `json:"workspace"`
	Mode          string `json:"mode,omitempty"`
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	Force         bool   `json:"force,omitempty"`
	AppointmentID string `json:"appointmentId,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

func NewAgentTask(payload AgentTaskPayload) (*asynq.Task, error) {
	// Gatekeeper uses a stripped payload (no fingerprint) for queue-level
	// uniqueness so short bursts per service collapse even when multiple
	// trigger sources produce different snapshots.
	if payload.Workspace == "gatekeeper" {
		stripped := AgentTaskPayload{
			Workspace:     payload.Workspace,
			TenantID:      payload.TenantID,
			LeadID:        payload.LeadID,
			LeadServiceID: payload.LeadServiceID,
		}
		data, err := json.Marshal(stripped)
		if err != nil {
			return nil, err
		}
		return asynq.NewTask(TaskAgentRun, data), nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAgentRun, data), nil
}

func ParseAgentTaskPayload(task *asynq.Task) (AgentTaskPayload, error) {
	var payload AgentTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AgentTaskPayload{}, err
	}
	return payload, nil
}

type AppointmentReminderPayload struct {
	AppointmentID  string `json:"appointmentId"`
	OrganizationID string `json:"organizationId"`
}

type TaskReminderPayload struct {
	ReminderID   string `json:"reminderId"`
	ScheduledFor string `json:"scheduledFor"`
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

type SubsidyAnalyzerJobPayload struct {
	JobID          string `json:"jobId"`
	TenantID       string `json:"tenantId"`
	UserID         string `json:"userId"`
	QuoteID        string `json:"quoteId"`
	OrganizationID string `json:"organizationId"`
}

type LogCallPayload struct {
	TenantID  string `json:"tenantId"`
	LeadID    string `json:"leadId"`
	ServiceID string `json:"serviceId"`
	UserID    string `json:"userId"`
	Summary   string `json:"summary"`
}

type PartnerOfferSummaryItemPayload struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
}

type PartnerOfferSummaryPayload struct {
	OfferID       string                           `json:"offerId"`
	TenantID      string                           `json:"tenantId"`
	LeadID        string                           `json:"leadId"`
	LeadServiceID string                           `json:"leadServiceId"`
	ServiceType   string                           `json:"serviceType"`
	Scope         *string                          `json:"scope,omitempty"`
	UrgencyLevel  *string                          `json:"urgencyLevel,omitempty"`
	Items         []PartnerOfferSummaryItemPayload `json:"items,omitempty"`
}

type PartnerOfferPDFPayload struct {
	OfferID  string `json:"offerId"`
	TenantID string `json:"tenantId"`
}

type WAAgentVoiceTranscriptionPayload struct {
	OrganizationID    string `json:"organizationId"`
	PhoneNumber       string `json:"phoneNumber"`
	ExternalMessageID string `json:"externalMessageId"`
	RequestID         string `json:"requestId,omitempty"`
	TraceID           string `json:"traceId,omitempty"`
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

// StaleLeadNotifyPayload carries the context needed to create re-engagement
// notifications for a single stale lead service.
type StaleLeadNotifyPayload struct {
	OrganizationID    string `json:"organizationId"`
	LeadID            string `json:"leadId"`
	LeadServiceID     string `json:"leadServiceId"`
	StaleReason       string `json:"staleReason"`
	ConsumerFirstName string `json:"consumerFirstName"`
	ConsumerLastName  string `json:"consumerLastName"`
	ConsumerPhone     string `json:"consumerPhone"`
	ServiceType       string `json:"serviceType"`
	PipelineStage     string `json:"pipelineStage"`
}

func NewAppointmentReminderTask(payload AppointmentReminderPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAppointmentReminder, data), nil
}

func NewTaskReminderTask(payload TaskReminderPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTaskReminder, data), nil
}

func ParseAppointmentReminderPayload(task *asynq.Task) (AppointmentReminderPayload, error) {
	var payload AppointmentReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AppointmentReminderPayload{}, err
	}
	return payload, nil
}

func ParseTaskReminderPayload(task *asynq.Task) (TaskReminderPayload, error) {
	var payload TaskReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return TaskReminderPayload{}, err
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

func NewSubsidyAnalyzerJobTask(payload SubsidyAnalyzerJobPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAnalyzeSubsidy, data), nil
}

func ParseSubsidyAnalyzerJobPayload(task *asynq.Task) (SubsidyAnalyzerJobPayload, error) {
	var payload SubsidyAnalyzerJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return SubsidyAnalyzerJobPayload{}, err
	}
	return payload, nil
}

func NewWAAgentVoiceTranscriptionTask(payload WAAgentVoiceTranscriptionPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskWAAgentVoiceTranscription, data), nil
}

func ParseWAAgentVoiceTranscriptionPayload(task *asynq.Task) (WAAgentVoiceTranscriptionPayload, error) {
	var payload WAAgentVoiceTranscriptionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return WAAgentVoiceTranscriptionPayload{}, err
	}
	return payload, nil
}

func NewPartnerOfferSummaryTask(payload PartnerOfferSummaryPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGeneratePartnerOfferSummary, data), nil
}

func ParsePartnerOfferSummaryPayload(task *asynq.Task) (PartnerOfferSummaryPayload, error) {
	var payload PartnerOfferSummaryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return PartnerOfferSummaryPayload{}, err
	}
	return payload, nil
}

func NewPartnerOfferPDFTask(payload PartnerOfferPDFPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGeneratePartnerOfferPDF, data), nil
}

func ParsePartnerOfferPDFPayload(task *asynq.Task) (PartnerOfferPDFPayload, error) {
	var payload PartnerOfferPDFPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return PartnerOfferPDFPayload{}, err
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

// StaleLeadReEngagePayload carries context for AI-powered re-engagement
// suggestion generation for a single stale lead service.
type StaleLeadReEngagePayload struct {
	OrganizationID string `json:"organizationId"`
	LeadID         string `json:"leadId"`
	LeadServiceID  string `json:"leadServiceId"`
	StaleReason    string `json:"staleReason"`
}

func NewStaleLeadReEngageTask(payload StaleLeadReEngagePayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskStaleLeadReEngage, data), nil
}

func ParseStaleLeadReEngagePayload(task *asynq.Task) (StaleLeadReEngagePayload, error) {
	var payload StaleLeadReEngagePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return StaleLeadReEngagePayload{}, err
	}
	return payload, nil
}

func NewStaleLeadNotifyTask(payload StaleLeadNotifyPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskStaleLeadNotify, data), nil
}

func ParseStaleLeadNotifyPayload(task *asynq.Task) (StaleLeadNotifyPayload, error) {
	var payload StaleLeadNotifyPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return StaleLeadNotifyPayload{}, err
	}
	return payload, nil
}
