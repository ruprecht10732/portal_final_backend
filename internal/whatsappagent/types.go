package whatsappagent

import (
	"context"
	"io"
	"time"

	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type QuotesReader interface {
	ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]QuoteSummary, error)
}

type QuoteWorkflowWriter interface {
	DraftQuote(ctx context.Context, orgID uuid.UUID, input DraftQuoteInput) (DraftQuoteOutput, error)
	GenerateQuote(ctx context.Context, orgID uuid.UUID, input GenerateQuoteInput) (GenerateQuoteOutput, error)
	GetQuotePDF(ctx context.Context, orgID uuid.UUID, input SendQuotePDFInput) (QuotePDFResult, error)
}

type AppointmentsReader interface {
	ListAppointmentsByOrganization(ctx context.Context, orgID uuid.UUID, from, to *time.Time) ([]AppointmentSummary, error)
}

type LeadSearchReader interface {
	SearchLeads(ctx context.Context, orgID uuid.UUID, query string, limit int) ([]LeadSearchResult, error)
}

type NavigationLinkReader interface {
	GetNavigationLink(ctx context.Context, orgID uuid.UUID, leadID string) (*NavigationLinkResult, error)
}

type LeadDetailsReader interface {
	GetLeadDetails(ctx context.Context, orgID uuid.UUID, leadID string) (*LeadDetailsResult, error)
}

type CatalogSearchReader interface {
	SearchProductMaterials(ctx context.Context, orgID uuid.UUID, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error)
}

type LeadMutationWriter interface {
	CreateLead(ctx context.Context, orgID uuid.UUID, input CreateLeadInput) (CreateLeadOutput, error)
	ResolveServiceID(ctx context.Context, leadID, organizationID uuid.UUID, requestedServiceID *uuid.UUID) (uuid.UUID, error)
	UpdateLeadDetails(ctx context.Context, orgID uuid.UUID, input UpdateLeadDetailsInput) ([]string, error)
	AskCustomerClarification(ctx context.Context, orgID uuid.UUID, input AskCustomerClarificationInput) error
	SaveNote(ctx context.Context, orgID uuid.UUID, input SaveNoteInput) error
	UpdateLeadStatus(ctx context.Context, orgID uuid.UUID, input UpdateStatusInput) (string, error)
}

type TaskWriter interface {
	CreateTask(ctx context.Context, orgID uuid.UUID, input CreateTaskInput) (CreateTaskOutput, error)
}

type TaskReader interface {
	GetLeadTasks(ctx context.Context, orgID uuid.UUID, input GetLeadTasksInput) (GetLeadTasksOutput, error)
}

type EnergyLabelReader interface {
	GetEnergyLabel(ctx context.Context, orgID uuid.UUID, input GetEnergyLabelInput) (GetEnergyLabelOutput, error)
}

type ISDECalculator interface {
	GetISDE(ctx context.Context, orgID uuid.UUID, input GetISDEInput) (GetISDEOutput, error)
}

type CurrentInboundPhotoAttacher interface {
	AttachCurrentWhatsAppPhoto(ctx context.Context, orgID uuid.UUID, input AttachCurrentWhatsAppPhotoInput, message CurrentInboundMessage) (AttachCurrentWhatsAppPhotoOutput, error)
}

type VisitSlotReader interface {
	GetAvailableVisitSlots(ctx context.Context, orgID uuid.UUID, startDate, endDate string, slotDuration int) ([]VisitSlotSummary, error)
}

type VisitMutationWriter interface {
	ScheduleVisit(ctx context.Context, orgID uuid.UUID, input ScheduleVisitInput) (*AppointmentSummary, error)
	RescheduleVisit(ctx context.Context, orgID uuid.UUID, input RescheduleVisitInput) (*AppointmentSummary, error)
	CancelVisit(ctx context.Context, orgID uuid.UUID, input CancelVisitInput) error
}

type PartnerPhoneReader interface {
	GetPartnerPhone(ctx context.Context, orgID, partnerID uuid.UUID) (*PartnerPhoneRecord, error)
}

type PartnerJobReader interface {
	ListPartnerJobs(ctx context.Context, orgID, partnerID uuid.UUID) ([]PartnerJobSummary, error)
	GetPartnerJobByService(ctx context.Context, orgID, partnerID, leadServiceID uuid.UUID) (*PartnerJobSummary, error)
	GetPartnerJobByAppointment(ctx context.Context, orgID, partnerID, appointmentID uuid.UUID) (*PartnerJobSummary, error)
	GetPartnerJobByLead(ctx context.Context, orgID, partnerID, leadID uuid.UUID) (*PartnerJobSummary, error)
}

type AppointmentVisitReportWriter interface {
	UpsertVisitReport(ctx context.Context, orgID, appointmentID uuid.UUID, input SaveMeasurementInput) error
}

type AppointmentStatusWriter interface {
	UpdateAppointmentStatus(ctx context.Context, orgID, appointmentID uuid.UUID, input UpdateAppointmentStatusInput) (*AppointmentSummary, error)
}

type InboxWriter interface {
	PersistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error
}

type InboxMessageSync interface {
	PersistIncomingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, phoneNumber, displayName, body string, externalMessageID *string, metadata []byte) error
	UpdateIncomingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, externalMessageID, body string, metadata []byte) error
}

type ObjectStorage interface {
	DownloadFile(ctx context.Context, bucket, fileKey string) (io.ReadCloser, error)
	UploadFile(ctx context.Context, bucket, folder, fileName, contentType string, reader io.Reader, size int64) (string, error)
	ValidateContentType(contentType string) error
	ValidateFileSize(sizeBytes int64) error
}

type WhatsAppTransport interface {
	SendMessage(ctx context.Context, deviceID string, phoneNumber string, message string) (whatsapp.SendResult, error)
	SendChatPresence(ctx context.Context, deviceID string, phoneNumber string, action string) error
	SendFile(ctx context.Context, deviceID string, input whatsapp.SendFileInput) (whatsapp.SendResult, error)
	DownloadMediaFile(ctx context.Context, deviceID string, messageID string, phoneNumber string, fallbackPhones ...string) (whatsapp.DownloadMediaFileResult, error)
}

type AgentConfigReader interface {
	GetAgentConfig(ctx context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error)
}

type AudioTranscriptionScheduler interface {
	EnqueueWAAgentVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) error
}

type ModuleConfig struct {
	ModelConfig      openaicompat.Config
	WebhookSecret    string
	StreamingEnabled bool
}

type ModuleDependencies struct {
	WhatsAppClient               *whatsapp.Client
	QuotesReader                 QuotesReader
	AppointmentsReader           AppointmentsReader
	LeadSearchReader             LeadSearchReader
	LeadDetailsReader            LeadDetailsReader
	NavigationLinkReader         NavigationLinkReader
	CatalogSearchReader          CatalogSearchReader
	LeadMutationWriter           LeadMutationWriter
	TaskWriter                   TaskWriter
	TaskReader                   TaskReader
	EnergyLabelReader            EnergyLabelReader
	ISDECalculator               ISDECalculator
	QuoteWorkflowWriter          QuoteWorkflowWriter
	CurrentInboundPhotoAttacher  CurrentInboundPhotoAttacher
	Storage                      ObjectStorage
	AttachmentBucket             string
	TranscriptionScheduler       AudioTranscriptionScheduler
	AudioTranscriber             AudioTranscriber
	InboxMessageSync             InboxMessageSync
	VisitSlotReader              VisitSlotReader
	VisitMutationWriter          VisitMutationWriter
	PartnerPhoneReader           PartnerPhoneReader
	PartnerJobReader             PartnerJobReader
	AppointmentVisitReportWriter AppointmentVisitReportWriter
	AppointmentStatusWriter      AppointmentStatusWriter
	RedisClient                  *redis.Client
	SessionRedis                 *redis.Client
	InboxWriter                  InboxWriter
	Logger                       *logger.Logger
}
