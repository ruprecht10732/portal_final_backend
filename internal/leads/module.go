// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the RAC_leads bounded context module implementing http.Module.
type Module struct {
	handler              *handler.Handler
	attachmentsHandler   *handler.AttachmentsHandler
	photoAnalysisHandler *handler.PhotoAnalysisHandler
	management           *management.Service
	notes                *notes.Service
	gatekeeper           *agent.Gatekeeper
	estimator            *agent.Estimator
	dispatcher           *agent.Dispatcher
	orchestrator         *Orchestrator
	photoAnalyzer        *agent.PhotoAnalyzer
	callLogger           *agent.CallLogger
	sse                  *sse.Service
	repo                 repository.LeadsRepository
	storage              storage.StorageService
	attachmentsBucket    string
	log                  *logger.Logger
	scorer               *scoring.Service
}

// NewModule creates and initializes the RAC_leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus, storageSvc storage.StorageService, val *validator.Validator, cfg *config.Config, log *logger.Logger) (*Module, error) {
	// Create shared repository
	repo := repository.New(pool)

	// Score service for lead scoring
	scorer := scoring.New(repo, log)

	photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, err := buildAgents(cfg, repo, storageSvc, scorer, eventBus)
	if err != nil {
		return nil, err
	}

	// SSE service for real-time notifications
	sseService := sse.New()

	// Subscribe to LeadCreated events to kick off gatekeeper triage
	subscribeLeadCreated(eventBus, repo, gatekeeper, log)

	// Create focused services (vertical slices)
	mapsSvc := maps.NewService(log)
	mgmtSvc := management.New(repo, eventBus, mapsSvc)
	mgmtSvc.SetLeadScorer(scorer)
	notesSvc := notes.New(repo)

	// Create orchestrator and event listeners
	orchestrator := NewOrchestrator(gatekeeper, estimator, dispatcher, repo, log)
	subscribeOrchestrator(eventBus, orchestrator)

	// Create handlers
	h, attachmentsHandler, photoAnalysisHandler := buildHandlers(buildHandlersDeps{
		MgmtSvc:       mgmtSvc,
		NotesSvc:      notesSvc,
		Gatekeeper:    gatekeeper,
		CallLogger:    callLogger,
		SSEService:    sseService,
		EventBus:      eventBus,
		Repo:          repo,
		StorageSvc:    storageSvc,
		Config:        cfg,
		Validator:     val,
		PhotoAnalyzer: photoAnalyzer,
	})

	return &Module{
		handler:              h,
		attachmentsHandler:   attachmentsHandler,
		photoAnalysisHandler: photoAnalysisHandler,
		management:           mgmtSvc,
		notes:                notesSvc,
		gatekeeper:           gatekeeper,
		estimator:            estimator,
		dispatcher:           dispatcher,
		orchestrator:         orchestrator,
		photoAnalyzer:        photoAnalyzer,
		callLogger:           callLogger,
		sse:                  sseService,
		repo:                 repo,
		storage:              storageSvc,
		attachmentsBucket:    cfg.GetMinioBucketLeadServiceAttachments(),
		log:                  log,
		scorer:               scorer,
	}, nil
}

func buildAgents(cfg *config.Config, repo repository.LeadsRepository, storageSvc storage.StorageService, scorer *scoring.Service, eventBus events.Bus) (*agent.PhotoAnalyzer, *agent.CallLogger, *agent.Gatekeeper, *agent.Estimator, *agent.Dispatcher, error) {
	photoAnalyzer, err := agent.NewPhotoAnalyzer(cfg.MoonshotAPIKey, repo)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	callLogger, err := agent.NewCallLogger(cfg.MoonshotAPIKey, repo, nil, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	gatekeeper, err := agent.NewGatekeeper(cfg.MoonshotAPIKey, repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	estimator, err := agent.NewEstimator(cfg.MoonshotAPIKey, repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	dispatcher, err := agent.NewDispatcher(cfg.MoonshotAPIKey, repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, nil
}

func subscribeLeadCreated(eventBus events.Bus, repo repository.LeadsRepository, gatekeeper *agent.Gatekeeper, log *logger.Logger) {
	eventBus.Subscribe(events.LeadCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadCreated)
		if !ok {
			return nil
		}

		go func() {
			bg := context.Background()
			service, err := repo.GetCurrentLeadService(bg, e.LeadID, e.TenantID)
			if err != nil {
				log.Error("gatekeeper: failed to load current service", "error", err, "leadId", e.LeadID)
				return
			}
			if err := gatekeeper.Run(bg, e.LeadID, service.ID, e.TenantID); err != nil {
				log.Error("gatekeeper run failed", "error", err, "leadId", e.LeadID)
			}
		}()

		return nil
	}))
}

func subscribeOrchestrator(eventBus events.Bus, orchestrator *Orchestrator) {
	eventBus.Subscribe(events.LeadDataChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadDataChanged)
		if !ok {
			return nil
		}
		orchestrator.OnDataChange(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PipelineStageChanged{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PipelineStageChanged)
		if !ok {
			return nil
		}
		orchestrator.OnStageChange(ctx, e)
		return nil
	}))
}

type buildHandlersDeps struct {
	MgmtSvc       *management.Service
	NotesSvc      *notes.Service
	Gatekeeper    *agent.Gatekeeper
	CallLogger    *agent.CallLogger
	SSEService    *sse.Service
	EventBus      events.Bus
	Repo          repository.LeadsRepository
	StorageSvc    storage.StorageService
	Config        *config.Config
	Validator     *validator.Validator
	PhotoAnalyzer *agent.PhotoAnalyzer
}

func buildHandlers(deps buildHandlersDeps) (*handler.Handler, *handler.AttachmentsHandler, *handler.PhotoAnalysisHandler) {
	notesHandler := handler.NewNotesHandler(deps.NotesSvc, deps.Repo, deps.EventBus, deps.Validator)
	attachmentsHandler := handler.NewAttachmentsHandler(deps.Repo, deps.StorageSvc, deps.Config.GetMinioBucketLeadServiceAttachments(), deps.Validator)
	photoAnalysisHandler := handler.NewPhotoAnalysisHandler(deps.PhotoAnalyzer, deps.Repo, deps.StorageSvc, deps.Config.GetMinioBucketLeadServiceAttachments(), deps.SSEService, deps.Validator)
	h := handler.New(handler.HandlerDeps{
		Mgmt:         deps.MgmtSvc,
		NotesHandler: notesHandler,
		Gatekeeper:   deps.Gatekeeper,
		CallLogger:   deps.CallLogger,
		SSE:          deps.SSEService,
		EventBus:     deps.EventBus,
		Repo:         deps.Repo,
		Validator:    deps.Validator,
	})

	return h, attachmentsHandler, photoAnalysisHandler
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "RAC_leads"
}

// ManagementService returns the lead management service for external use.
func (m *Module) ManagementService() *management.Service {
	return m.management
}

// NotesService returns the lead notes service for external use.
func (m *Module) NotesService() *notes.Service {
	return m.notes
}

// CallLogger returns the call logger agent for external use.
func (m *Module) CallLogger() *agent.CallLogger {
	return m.callLogger
}

// PhotoAnalyzer returns the photo analyzer agent for external use.
func (m *Module) PhotoAnalyzer() *agent.PhotoAnalyzer {
	return m.photoAnalyzer
}

// SSE returns the SSE service for external use.
func (m *Module) SSE() *sse.Service {
	return m.sse
}

// Repository returns the RAC_leads repository for external use.
func (m *Module) Repository() repository.LeadsRepository {
	return m.repo
}

// SetAppointmentBooker sets the appointment booker on the CallLogger.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetAppointmentBooker(booker ports.AppointmentBooker) {
	m.callLogger.SetAppointmentBooker(booker)
}

// SetEnergyLabelEnricher sets the energy label enricher on the management service.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetEnergyLabelEnricher(enricher ports.EnergyLabelEnricher) {
	m.management.SetEnergyLabelEnricher(enricher)
}

// SetLeadEnricher sets the lead enrichment provider.
func (m *Module) SetLeadEnricher(enricher ports.LeadEnricher) {
	m.management.SetLeadEnricher(enricher)
}

// SetLeadScorer sets the scoring service for lead updates.
func (m *Module) SetLeadScorer(scorer *scoring.Service) {
	m.management.SetLeadScorer(scorer)
	m.scorer = scorer
}

// RegisterRoutes mounts RAC_leads routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// All RAC_leads routes require authentication
	leadsGroup := ctx.Protected.Group("/leads")
	m.handler.RegisterRoutes(leadsGroup)

	// Attachment routes: /RAC_leads/:id/services/:serviceId/attachments
	attachmentsGroup := leadsGroup.Group("/:id/services/:serviceId/attachments")
	m.attachmentsHandler.RegisterRoutes(attachmentsGroup)

	// Photo analysis routes: /RAC_leads/:id/services/:serviceId/...
	photoAnalysisGroup := leadsGroup.Group("/:id/services/:serviceId")
	m.photoAnalysisHandler.RegisterRoutes(photoAnalysisGroup)

	// SSE endpoint for real-time notifications (user-specific)
	ctx.Protected.GET("/events", m.sseHandler())
}

// sseHandler returns the SSE handler with user ID extraction
func (m *Module) sseHandler() func(c *gin.Context) {
	return m.sse.Handler(
		func(c *gin.Context) (uuid.UUID, bool) {
			id := httpkit.GetIdentity(c)
			if id == nil || !id.IsAuthenticated() {
				return uuid.UUID{}, false
			}
			return id.UserID(), true
		},
		func(c *gin.Context) (uuid.UUID, bool) {
			id := httpkit.GetIdentity(c)
			if id == nil || !id.IsAuthenticated() {
				return uuid.UUID{}, false
			}
			tenantID := id.TenantID()
			if tenantID == nil {
				return uuid.UUID{}, false
			}
			return *tenantID, true
		},
	)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
