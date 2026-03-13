package waagent

import (
	"context"
	"time"

	apphttp "portal_final_backend/internal/http"
	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"
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
	QuoteNumber string `json:"quote_number"`
	ClientName  string `json:"client_name"`
	ClientPhone string `json:"client_phone,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	ClientCity  string `json:"client_city,omitempty"`
	TotalCents  int64  `json:"total_cents"`
	Status      string `json:"status"`
	Summary     string `json:"summary,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// AppointmentSummary is a simplified appointment representation returned to the LLM.
type AppointmentSummary struct {
	AppointmentID string `json:"appointment_id,omitempty"`
	LeadID        string `json:"lead_id,omitempty"`
	LeadServiceID string `json:"lead_service_id,omitempty"`
	AssignedUserID string `json:"assigned_user_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Status      string `json:"status"`
	Location    string `json:"location,omitempty"`
}

// QuotesReader lists quotes for an organization.
type QuotesReader interface {
	ListQuotesByOrganization(ctx context.Context, orgID uuid.UUID, status *string) ([]QuoteSummary, error)
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
	UpdateLeadDetails(ctx context.Context, orgID uuid.UUID, input UpdateLeadDetailsInput) ([]string, error)
	AskCustomerClarification(ctx context.Context, orgID uuid.UUID, input AskCustomerClarificationInput) error
	SaveNote(ctx context.Context, orgID uuid.UUID, input SaveNoteInput) error
	UpdateLeadStatus(ctx context.Context, orgID uuid.UUID, input UpdateStatusInput) (string, error)
}

type VisitSlotReader interface {
	GetAvailableVisitSlots(ctx context.Context, orgID uuid.UUID, startDate, endDate string, slotDuration int) ([]VisitSlotSummary, error)
}

type VisitMutationWriter interface {
	ScheduleVisit(ctx context.Context, orgID uuid.UUID, input ScheduleVisitInput) (*AppointmentSummary, error)
	RescheduleVisit(ctx context.Context, orgID uuid.UUID, input RescheduleVisitInput) (*AppointmentSummary, error)
	CancelVisit(ctx context.Context, orgID uuid.UUID, input CancelVisitInput) error
}

// InboxWriter persists outgoing messages to the operator inbox.
type InboxWriter interface {
	PersistOutgoingWhatsAppMessage(ctx context.Context, organizationID uuid.UUID, leadID *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error
}

// ModuleConfig holds waagent configuration.
type ModuleConfig struct {
	MoonshotAPIKey string
	LLMModel       string
	WebhookSecret  string
}

// ModuleDependencies groups external waagent dependencies to keep constructor size manageable.
type ModuleDependencies struct {
	WhatsAppClient     *whatsapp.Client
	QuotesReader       QuotesReader
	AppointmentsReader AppointmentsReader
	LeadSearchReader   LeadSearchReader
	LeadDetailsReader  LeadDetailsReader
	NavigationLinkReader NavigationLinkReader
	CatalogSearchReader CatalogSearchReader
	LeadMutationWriter LeadMutationWriter
	VisitSlotReader    VisitSlotReader
	VisitMutationWriter VisitMutationWriter
	RedisClient        *redis.Client
	InboxWriter        InboxWriter
	Logger             *logger.Logger
}

// Module is the waagent bounded context module.
type Module struct {
	service       *Service
	deviceHandler *DeviceHandler
	phoneHandler  *PhoneHandler
}

// NewModule creates and initialises the waagent module.
func NewModule(pool *pgxpool.Pool, cfg ModuleConfig, deps ModuleDependencies) (*Module, error) {

	queries := waagentdb.New(pool)
	hintStore := NewConversationLeadHintStore()

	toolHandler := &ToolHandler{
		quotesReader:       deps.QuotesReader,
		appointmentsReader: deps.AppointmentsReader,
		leadSearchReader:   deps.LeadSearchReader,
		leadHintStore:      hintStore,
		leadDetailsReader:  deps.LeadDetailsReader,
		navigationLinkReader: deps.NavigationLinkReader,
		catalogSearchReader: deps.CatalogSearchReader,
		leadMutationWriter: deps.LeadMutationWriter,
		visitSlotReader:    deps.VisitSlotReader,
		visitMutationWriter: deps.VisitMutationWriter,
	}

	agent, err := NewAgent(moonshot.Config{
		APIKey:          cfg.MoonshotAPIKey,
		Model:           cfg.LLMModel,
		DisableThinking: true,
	}, toolHandler)
	if err != nil {
		return nil, err
	}

	sender := &Sender{
		client:      deps.WhatsAppClient,
		queries:     queries,
		inboxWriter: deps.InboxWriter,
		log:         deps.Logger,
	}

	rateLimiter := NewRateLimiter(deps.RedisClient, deps.Logger)

	svc := &Service{
		queries:           queries,
		agent:             agent,
		sender:            sender,
		rateLimiter:       rateLimiter,
		leadHintStore:     hintStore,
		leadDetailsReader: deps.LeadDetailsReader,
		log:               deps.Logger,
	}

	return &Module{
		service:       svc,
		deviceHandler: &DeviceHandler{queries: queries, waClient: deps.WhatsAppClient, webhookSecret: cfg.WebhookSecret},
		phoneHandler:  &PhoneHandler{queries: queries},
	}, nil
}

// Service returns the waagent service for webhook integration.
func (m *Module) Service() *Service { return m.service }

// RegisterRoutes mounts waagent routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Superadmin: global agent device management
	agentDevice := ctx.SuperAdmin.Group("/whatsapp-agent")
	m.deviceHandler.RegisterSuperAdminRoutes(agentDevice)

	// Org admin: phone-to-org registration
	agentMembers := ctx.Admin.Group("/organizations/me/whatsapp-agent/members")
	m.phoneHandler.RegisterAdminRoutes(agentMembers)
}

// Name returns the module identifier.
func (m *Module) Name() string { return "waagent" }
