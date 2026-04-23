package appointments

import (
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/appointments/handler"
	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module represents the appointments domain module.
type Module struct {
	Service *service.Service
	handler *handler.Handler
	sse     *sse.Service
}

type Dependencies struct {
	Pool              *pgxpool.Pool
	Validator         *validator.Validator
	LeadAssigner      service.LeadAssigner
	EmailSender       email.Sender
	EventBus          events.Bus
	ReminderScheduler scheduler.ReminderScheduler
	Storage           storage.StorageService
	TimelineRecorder  leadsrepo.TimelineEventStore
	AttachmentBucket  string
}

// NewModule creates a new appointments module with all dependencies wired.
func NewModule(deps Dependencies) *Module {
	repo := repository.New(deps.Pool)
	svc := service.New(service.Dependencies{
		Repo:              repo,
		LeadAssigner:      deps.LeadAssigner,
		EmailSender:       deps.EmailSender,
		EventBus:          deps.EventBus,
		ReminderScheduler: deps.ReminderScheduler,
		Storage:           deps.Storage,
		AttachmentBucket:  deps.AttachmentBucket,
		TimelineRecorder:  deps.TimelineRecorder,
	})

	return &Module{
		Service: svc,
		handler: handler.New(svc, deps.Validator),
	}
}

// SetSSE sets the SSE service for real-time appointment event broadcasting.
func (m *Module) SetSSE(sseService *sse.Service) {
	m.sse = sseService
	m.Service.SetSSE(sseService)
}

// Name returns the module name for logging purposes.
func (m *Module) Name() string {
	return "appointments"
}

// RegisterRoutes registers the module's routes under /api/appointments.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	m.handler.RegisterRoutes(ctx.Protected.Group("/appointments"))
}

// Compile-time check that Module implements apphttp.Module.
var _ apphttp.Module = (*Module)(nil)
