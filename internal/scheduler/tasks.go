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
const TaskGeneratePartnerOfferSummary = "partners.offer.generate_summary"
const TaskRunGatekeeper = "leads.gatekeeper.run"
const TaskRunEstimator = "leads.estimator.run"
const TaskRunDispatcher = "leads.dispatcher.run"
const TaskAnalyzePhotos = "leads.photo_analysis.run"
const TaskAuditVisitReport = "leads.audit.visit_report"
const TaskAuditCallLog = "leads.audit.call_log"
const TaskWAAgentVoiceTranscription = "waagent.voice_transcription.run"
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

type GatekeeperRunPayload struct {
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

type EstimatorRunPayload struct {
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	Force         bool   `json:"force,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

type DispatcherRunPayload struct {
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

type PhotoAnalysisPayload struct {
	TenantID      string  `json:"tenantId"`
	LeadID        string  `json:"leadId"`
	LeadServiceID string  `json:"leadServiceId"`
	UserID        *string `json:"userId,omitempty"`
	ContextInfo   string  `json:"contextInfo,omitempty"`
}

type AuditVisitReportPayload struct {
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	AppointmentID string `json:"appointmentId"`
}

type AuditCallLogPayload struct {
	TenantID      string `json:"tenantId"`
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
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

func NewGatekeeperRunTask(payload GatekeeperRunPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskRunGatekeeper, data), nil
}

func ParseGatekeeperRunPayload(task *asynq.Task) (GatekeeperRunPayload, error) {
	var payload GatekeeperRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return GatekeeperRunPayload{}, err
	}
	return payload, nil
}

func NewEstimatorRunTask(payload EstimatorRunPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskRunEstimator, data), nil
}

func ParseEstimatorRunPayload(task *asynq.Task) (EstimatorRunPayload, error) {
	var payload EstimatorRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return EstimatorRunPayload{}, err
	}
	return payload, nil
}

func NewDispatcherRunTask(payload DispatcherRunPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskRunDispatcher, data), nil
}

func ParseDispatcherRunPayload(task *asynq.Task) (DispatcherRunPayload, error) {
	var payload DispatcherRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return DispatcherRunPayload{}, err
	}
	return payload, nil
}

func NewPhotoAnalysisTask(payload PhotoAnalysisPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAnalyzePhotos, data), nil
}

func ParsePhotoAnalysisPayload(task *asynq.Task) (PhotoAnalysisPayload, error) {
	var payload PhotoAnalysisPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return PhotoAnalysisPayload{}, err
	}
	return payload, nil
}

func NewAuditVisitReportTask(payload AuditVisitReportPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAuditVisitReport, data), nil
}

func ParseAuditVisitReportPayload(task *asynq.Task) (AuditVisitReportPayload, error) {
	var payload AuditVisitReportPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AuditVisitReportPayload{}, err
	}
	return payload, nil
}

func NewAuditCallLogTask(payload AuditCallLogPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAuditCallLog, data), nil
}

func ParseAuditCallLogPayload(task *asynq.Task) (AuditCallLogPayload, error) {
	var payload AuditCallLogPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return AuditCallLogPayload{}, err
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
