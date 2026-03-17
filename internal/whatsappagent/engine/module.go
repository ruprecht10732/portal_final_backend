package engine

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/ai/moonshot"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// QuoteSummary is a simplified quote representation returned to the LLM.
type QuoteSummary struct {
	QuoteID       string `json:"quote_id,omitempty"`
	LeadID        string `json:"lead_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	QuoteNumber   string `json:"quote_number"`
	ClientName    string `json:"client_name"`
	ClientPhone   string `json:"client_phone,omitempty"`
	ClientEmail   string `json:"client_email,omitempty"`
	ClientCity    string `json:"client_city,omitempty"`
	TotalCents    int64  `json:"total_cents"`
	Status        string `json:"status"`
	Summary       string `json:"summary,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// AppointmentSummary is a simplified appointment representation returned to the LLM.
type AppointmentSummary struct {
	AppointmentID  string `json:"appointment_id,omitempty"`
	LeadID         string `json:"lead_id,omitempty"`
	LeadServiceID  string `json:"lead_service_id,omitempty"`
	AssignedUserID string `json:"assigned_user_id,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Status         string `json:"status"`
	Location       string `json:"location,omitempty"`
}

// QuotesReader lists quotes for an organization.
type QuotesReader interface {
	ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]QuoteSummary, error)
}

type QuoteWorkflowWriter interface {
	DraftQuote(ctx context.Context, orgID uuid.UUID, input DraftQuoteInput) (DraftQuoteOutput, error)
	GenerateQuote(ctx context.Context, orgID uuid.UUID, input GenerateQuoteInput) (GenerateQuoteOutput, error)
	GetQuotePDF(ctx context.Context, orgID uuid.UUID, input SendQuotePDFInput) (QuotePDFResult, error)
}

// AppointmentsReader lists appointments for an organization.
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

type CurrentInboundMessage struct {
	ExternalMessageID string
	PhoneNumber       string
	DisplayName       string
	Body              string
	Metadata          []byte
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

type PartnerPhoneRecord struct {
	PartnerID    uuid.UUID
	DisplayName  string
	PhoneNumber  string
	BusinessName string
}

type PartnerPhoneReader interface {
	GetPartnerPhone(ctx context.Context, orgID, partnerID uuid.UUID) (*PartnerPhoneRecord, error)
}

type PartnerJobSummary struct {
	OfferID            string `json:"offer_id,omitempty"`
	PartnerID          string `json:"partner_id,omitempty"`
	LeadID             string `json:"lead_id,omitempty"`
	LeadServiceID      string `json:"lead_service_id,omitempty"`
	AppointmentID      string `json:"appointment_id,omitempty"`
	CustomerName       string `json:"customer_name,omitempty"`
	ServiceType        string `json:"service_type,omitempty"`
	City               string `json:"city,omitempty"`
	AppointmentTitle   string `json:"appointment_title,omitempty"`
	AppointmentStatus  string `json:"appointment_status,omitempty"`
	AppointmentStart   string `json:"appointment_start,omitempty"`
	AppointmentEnd     string `json:"appointment_end,omitempty"`
	DestinationAddress string `json:"destination_address,omitempty"`
	VakmanPriceCents   int64  `json:"vakman_price_cents,omitempty"`
	OfferStatus        string `json:"offer_status,omitempty"`
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

// InboxWriter persists outgoing messages to the operator inbox.
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

type AudioTranscriptionInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

type AudioTranscriptionResult struct {
	Text       string
	Language   string
	Confidence *float64
}

type AudioTranscriber interface {
	Name() string
	Transcribe(ctx context.Context, input AudioTranscriptionInput) (AudioTranscriptionResult, error)
}

type AudioTranscriptionScheduler interface {
	EnqueueWAAgentVoiceTranscription(ctx context.Context, payload scheduler.WAAgentVoiceTranscriptionPayload) error
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

// ModuleConfig holds whatsappagent configuration.
type ModuleConfig struct {
	MoonshotAPIKey string
	LLMModel       string
	WebhookSecret  string
}

// ModuleDependencies groups external whatsappagent dependencies to keep constructor size manageable.
type ModuleDependencies struct {
	WhatsAppClient               *whatsapp.Client
	QuotesReader                 QuotesReader
	AppointmentsReader           AppointmentsReader
	LeadSearchReader             LeadSearchReader
	LeadDetailsReader            LeadDetailsReader
	NavigationLinkReader         NavigationLinkReader
	CatalogSearchReader          CatalogSearchReader
	LeadMutationWriter           LeadMutationWriter
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
	InboxWriter                  InboxWriter
	Logger                       *logger.Logger
}

// Module is the whatsappagent bounded context module.
type Module struct {
	service       *Service
	deviceHandler *DeviceHandler
	phoneHandler  *PhoneHandler
}

func appendMissingDependency(missing []string, condition bool, name string) []string {
	if condition {
		return append(missing, name)
	}
	return missing
}

func collectCoreDependencyErrors(pool *pgxpool.Pool, deps ModuleDependencies) []string {
	missing := make([]string, 0, 2)
	missing = appendMissingDependency(missing, pool == nil, "database pool")
	missing = appendMissingDependency(missing, deps.Logger == nil, "logger")
	return missing
}

func collectRuntimeDependencyErrors(cfg ModuleConfig, deps ModuleDependencies) []string {
	missing := make([]string, 0, 20)
	missing = appendMissingDependency(missing, strings.TrimSpace(cfg.MoonshotAPIKey) == "", "moonshot api key")
	missing = appendMissingDependency(missing, strings.TrimSpace(cfg.LLMModel) == "", "llm model")
	missing = appendMissingDependency(missing, deps.WhatsAppClient == nil, "whatsapp client")
	missing = appendMissingDependency(missing, deps.QuotesReader == nil, "quotes reader")
	missing = appendMissingDependency(missing, deps.AppointmentsReader == nil, "appointments reader")
	missing = appendMissingDependency(missing, deps.LeadSearchReader == nil, "lead search reader")
	missing = appendMissingDependency(missing, deps.LeadDetailsReader == nil, "lead details reader")
	missing = appendMissingDependency(missing, deps.NavigationLinkReader == nil, "navigation link reader")
	missing = appendMissingDependency(missing, deps.CatalogSearchReader == nil, "catalog search reader")
	missing = appendMissingDependency(missing, deps.LeadMutationWriter == nil, "lead mutation writer")
	missing = appendMissingDependency(missing, deps.QuoteWorkflowWriter == nil, "quote workflow writer")
	missing = appendMissingDependency(missing, deps.CurrentInboundPhotoAttacher == nil, "current inbound photo attacher")
	missing = appendMissingDependency(missing, deps.InboxMessageSync == nil, "inbox message sync")
	missing = appendMissingDependency(missing, deps.VisitSlotReader == nil, "visit slot reader")
	missing = appendMissingDependency(missing, deps.VisitMutationWriter == nil, "visit mutation writer")
	missing = appendMissingDependency(missing, deps.PartnerPhoneReader == nil, "partner phone reader")
	missing = appendMissingDependency(missing, deps.PartnerJobReader == nil, "partner job reader")
	missing = appendMissingDependency(missing, deps.AppointmentVisitReportWriter == nil, "appointment visit report writer")
	missing = appendMissingDependency(missing, deps.AppointmentStatusWriter == nil, "appointment status writer")
	missing = appendMissingDependency(missing, deps.RedisClient == nil, "redis client")
	missing = appendMissingDependency(missing, deps.InboxWriter == nil, "inbox writer")
	return missing
}

func collectAudioDependencyErrors(deps ModuleDependencies) []string {
	hasAudioConfig := deps.Storage != nil || strings.TrimSpace(deps.AttachmentBucket) != "" || deps.TranscriptionScheduler != nil || deps.AudioTranscriber != nil
	if !hasAudioConfig {
		return nil
	}
	missing := make([]string, 0, 4)
	missing = appendMissingDependency(missing, deps.Storage == nil, "storage")
	missing = appendMissingDependency(missing, strings.TrimSpace(deps.AttachmentBucket) == "", "attachment bucket")
	missing = appendMissingDependency(missing, deps.TranscriptionScheduler == nil, "transcription scheduler")
	missing = appendMissingDependency(missing, deps.AudioTranscriber == nil, "audio transcriber")
	if len(missing) == 0 {
		return nil
	}
	return missing
}

func validateModuleDependencies(pool *pgxpool.Pool, deps ModuleDependencies) error {
	missing := collectCoreDependencyErrors(pool, deps)
	if len(missing) > 0 {
		return fmt.Errorf("whatsappagent: invalid module configuration: missing %s", strings.Join(missing, ", "))
	}

	return nil
}

func validateRuntimeDependencies(cfg ModuleConfig, deps ModuleDependencies) error {
	missing := collectRuntimeDependencyErrors(cfg, deps)
	if len(missing) > 0 {
		return fmt.Errorf("whatsappagent: invalid runtime configuration: missing %s", strings.Join(missing, ", "))
	}

	audioMissing := collectAudioDependencyErrors(deps)
	if len(audioMissing) > 0 {
		return fmt.Errorf("whatsappagent: invalid audio transcription configuration: missing %s", strings.Join(audioMissing, ", "))
	}

	return nil
}

// NewModule creates and initialises the whatsappagent module.
func NewModule(pool *pgxpool.Pool, cfg ModuleConfig, deps ModuleDependencies) (*Module, error) {
	if err := validateModuleDependencies(pool, deps); err != nil {
		return nil, err
	}

	queries := whatsappagentdb.New(pool)
	module := &Module{
		phoneHandler: &PhoneHandler{queries: queries, partnerPhoneReader: deps.PartnerPhoneReader},
	}
	if deps.WhatsAppClient != nil {
		module.deviceHandler = &DeviceHandler{queries: queries, waClient: deps.WhatsAppClient, webhookSecret: cfg.WebhookSecret}
	}

	if err := validateRuntimeDependencies(cfg, deps); err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("whatsappagent: AI runtime unavailable; admin membership routes remain enabled", "error", err)
		}
		return module, nil
	}

	hintStore := NewRedisConversationLeadHintStore(deps.RedisClient, deps.Logger)
	sender := &Sender{
		client:      deps.WhatsAppClient,
		queries:     queries,
		inboxWriter: deps.InboxWriter,
		log:         deps.Logger,
	}

	toolHandler := &ToolHandler{
		quotesReader:                 deps.QuotesReader,
		appointmentsReader:           deps.AppointmentsReader,
		leadSearchReader:             deps.LeadSearchReader,
		leadHintStore:                hintStore,
		leadDetailsReader:            deps.LeadDetailsReader,
		navigationLinkReader:         deps.NavigationLinkReader,
		catalogSearchReader:          deps.CatalogSearchReader,
		leadMutationWriter:           deps.LeadMutationWriter,
		quoteWorkflowWriter:          deps.QuoteWorkflowWriter,
		currentInboundPhotoAttacher:  deps.CurrentInboundPhotoAttacher,
		sender:                       sender,
		visitSlotReader:              deps.VisitSlotReader,
		visitMutationWriter:          deps.VisitMutationWriter,
		partnerJobReader:             deps.PartnerJobReader,
		appointmentVisitReportWriter: deps.AppointmentVisitReportWriter,
		appointmentStatusWriter:      deps.AppointmentStatusWriter,
	}

	agent, err := NewAgent(moonshot.Config{
		APIKey:          cfg.MoonshotAPIKey,
		Model:           cfg.LLMModel,
		DisableThinking: false,
	}, toolHandler, deps.Logger)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("whatsappagent: failed to initialize AI runtime; admin membership routes remain enabled", "error", err)
		}
		return module, nil
	}

	rateLimiter := NewRateLimiter(deps.RedisClient, deps.Logger)

	svc := &Service{
		queries:                queries,
		agent:                  agent,
		sender:                 sender,
		rateLimiter:            rateLimiter,
		leadHintStore:          hintStore,
		leadDetailsReader:      deps.LeadDetailsReader,
		storage:                deps.Storage,
		attachmentBucket:       deps.AttachmentBucket,
		transcriptionScheduler: deps.TranscriptionScheduler,
		transcriber:            deps.AudioTranscriber,
		inboxSync:              deps.InboxMessageSync,
		log:                    deps.Logger,
	}

	module.service = svc
	return module, nil
}

// Service returns the whatsappagent service for webhook integration.
func (m *Module) Service() *Service { return m.service }

// RegisterAdminRoutes mounts only the org-admin whatsappagent routes.
func (m *Module) RegisterAdminRoutes(ctx *apphttp.RouterContext) {
	if m == nil || ctx == nil {
		return
	}
	if m.phoneHandler != nil {
		agentMembers := ctx.Admin.Group("/organizations/me/whatsapp-agent/members")
		m.phoneHandler.RegisterAdminRoutes(agentMembers)
	}
}

// RegisterRoutes mounts whatsappagent routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	if m == nil || ctx == nil {
		return
	}
	// Superadmin: global agent device management
	if m.deviceHandler != nil {
		agentDevice := ctx.SuperAdmin.Group("/whatsapp-agent")
		m.deviceHandler.RegisterSuperAdminRoutes(agentDevice)
	}

	m.RegisterAdminRoutes(ctx)
}

// Name returns the module identifier.
func (m *Module) Name() string { return "whatsappagent" }
