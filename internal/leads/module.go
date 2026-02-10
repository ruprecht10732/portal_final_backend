// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"
	"time"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/qdrant"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the RAC_leads bounded context module implementing http.Module.
type Module struct {
	handler               *handler.Handler
	attachmentsHandler    *handler.AttachmentsHandler
	photoAnalysisHandler  *handler.PhotoAnalysisHandler
	publicHandler         *handler.PublicHandler
	management            *management.Service
	notes                 *notes.Service
	gatekeeper            *agent.Gatekeeper
	estimator             *agent.Estimator
	dispatcher            *agent.Dispatcher
	orchestrator          *Orchestrator
	photoAnalyzer         *agent.PhotoAnalyzer
	callLogger            *agent.CallLogger
	quoteGenerator        *agent.QuoteGenerator
	offerSummaryGenerator *agent.OfferSummaryGenerator
	sse                   *sse.Service
	repo                  repository.LeadsRepository
	storage               storage.StorageService
	attachmentsBucket     string
	log                   *logger.Logger
	scorer                *scoring.Service
}

// NewModule creates and initializes the RAC_leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus, storageSvc storage.StorageService, val *validator.Validator, cfg *config.Config, log *logger.Logger) (*Module, error) {
	// Create shared repository
	repo := repository.New(pool)

	// Score service for lead scoring
	scorer := scoring.New(repo, log)

	photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, quoteGenerator, offerSummaryGenerator, err := buildAgents(cfg, repo, storageSvc, scorer, eventBus)
	if err != nil {
		return nil, err
	}

	// SSE service for real-time notifications
	sseService := sse.New()

	// Subscribe to LeadCreated and LeadServiceAdded events to kick off gatekeeper triage
	subscribeLeadCreated(eventBus, repo, gatekeeper, log)
	subscribeLeadServiceAdded(eventBus, repo, gatekeeper, log)

	// Create focused services (vertical slices)
	mapsSvc := maps.NewService(log)
	mgmtSvc := management.New(repo, eventBus, mapsSvc)
	mgmtSvc.SetLeadScorer(scorer)
	notesSvc := notes.New(repo)

	// Create orchestrator and event listeners
	orchestrator := NewOrchestrator(gatekeeper, estimator, dispatcher, repo, eventBus, sseService, log)
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

	photoBatcher := newPhotoAnalysisBatcher(photoAnalysisHandler, 60*time.Second, log)
	subscribeAttachmentUploaded(eventBus, repo, photoBatcher, log)
	publicHandler := handler.NewPublicHandler(repo, eventBus, sseService, storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), val)

	return &Module{
		handler:               h,
		attachmentsHandler:    attachmentsHandler,
		photoAnalysisHandler:  photoAnalysisHandler,
		publicHandler:         publicHandler,
		management:            mgmtSvc,
		notes:                 notesSvc,
		gatekeeper:            gatekeeper,
		estimator:             estimator,
		dispatcher:            dispatcher,
		orchestrator:          orchestrator,
		photoAnalyzer:         photoAnalyzer,
		callLogger:            callLogger,
		quoteGenerator:        quoteGenerator,
		offerSummaryGenerator: offerSummaryGenerator,
		sse:                   sseService,
		repo:                  repo,
		storage:               storageSvc,
		attachmentsBucket:     cfg.GetMinioBucketLeadServiceAttachments(),
		log:                   log,
		scorer:                scorer,
	}, nil
}

func buildAgents(cfg *config.Config, repo repository.LeadsRepository, storageSvc storage.StorageService, scorer *scoring.Service, eventBus events.Bus) (*agent.PhotoAnalyzer, *agent.CallLogger, *agent.Gatekeeper, *agent.Estimator, *agent.Dispatcher, *agent.QuoteGenerator, *agent.OfferSummaryGenerator, error) {
	_ = storageSvc
	_ = scorer
	photoAnalyzer, err := agent.NewPhotoAnalyzer(cfg.MoonshotAPIKey, repo)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	callLogger, err := agent.NewCallLogger(cfg.MoonshotAPIKey, repo, nil, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	gatekeeper, err := agent.NewGatekeeper(cfg.MoonshotAPIKey, repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Create embedding and qdrant clients if configured
	var embeddingClient *embeddings.Client
	var qdrantClient *qdrant.Client
	var catalogQdrantClient *qdrant.Client

	if cfg.IsEmbeddingEnabled() {
		embeddingClient = embeddings.NewClient(embeddings.Config{
			BaseURL: cfg.GetEmbeddingAPIURL(),
			APIKey:  cfg.GetEmbeddingAPIKey(),
		})
	}

	if cfg.IsQdrantEnabled() {
		qdrantClient = qdrant.NewClient(qdrant.Config{
			BaseURL:    cfg.GetQdrantURL(),
			APIKey:     cfg.GetQdrantAPIKey(),
			Collection: cfg.GetQdrantCollection(),
		})
	}

	if cfg.GetQdrantURL() != "" && cfg.GetCatalogEmbeddingCollection() != "" {
		catalogQdrantClient = qdrant.NewClient(qdrant.Config{
			BaseURL:    cfg.GetQdrantURL(),
			APIKey:     cfg.GetQdrantAPIKey(),
			Collection: cfg.GetCatalogEmbeddingCollection(),
		})
	}

	estimator, err := agent.NewEstimator(agent.EstimatorConfig{
		APIKey:              cfg.MoonshotAPIKey,
		Repo:                repo,
		EventBus:            eventBus,
		EmbeddingClient:     embeddingClient,
		QdrantClient:        qdrantClient,
		CatalogQdrantClient: catalogQdrantClient,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	dispatcher, err := agent.NewDispatcher(cfg.MoonshotAPIKey, repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	quoteGenerator, err := agent.NewQuoteGenerator(agent.QuoteGeneratorConfig{
		APIKey:              cfg.MoonshotAPIKey,
		Repo:                repo,
		EventBus:            eventBus,
		EmbeddingClient:     embeddingClient,
		QdrantClient:        qdrantClient,
		CatalogQdrantClient: catalogQdrantClient,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	offerSummaryGenerator, err := agent.NewOfferSummaryGenerator(cfg.MoonshotAPIKey)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	return photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, quoteGenerator, offerSummaryGenerator, nil
}

// OfferSummaryGenerator exposes the AI summary generator for partner offers.
func (m *Module) OfferSummaryGenerator() ports.OfferSummaryGenerator {
	return m.offerSummaryGenerator
}

func subscribeLeadCreated(eventBus events.Bus, repo repository.LeadsRepository, gatekeeper *agent.Gatekeeper, log *logger.Logger) {
	eventBus.Subscribe(events.LeadCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadCreated)
		if !ok {
			return nil
		}

		go func() {
			bg := context.Background()

			// Terminal check: verify the service is not already in a terminal state
			service, err := repo.GetLeadServiceByID(bg, e.LeadServiceID, e.TenantID)
			if err != nil {
				log.Error("gatekeeper: failed to load service", "error", err, "leadId", e.LeadID, "serviceId", e.LeadServiceID)
				return
			}
			if domain.IsTerminal(service.Status, service.PipelineStage) {
				log.Info("gatekeeper: skipping terminal service on lead created", "leadId", e.LeadID, "serviceId", e.LeadServiceID)
				return
			}

			if err := gatekeeper.Run(bg, e.LeadID, e.LeadServiceID, e.TenantID); err != nil {
				log.Error("gatekeeper run failed", "error", err, "leadId", e.LeadID)
			}
		}()

		return nil
	}))
}

func subscribeLeadServiceAdded(eventBus events.Bus, repo repository.LeadsRepository, gatekeeper *agent.Gatekeeper, log *logger.Logger) {
	eventBus.Subscribe(events.LeadServiceAdded{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadServiceAdded)
		if !ok {
			return nil
		}

		go func() {
			bg := context.Background()

			// Terminal check: verify the service is not already in a terminal state
			service, err := repo.GetLeadServiceByID(bg, e.LeadServiceID, e.TenantID)
			if err != nil {
				log.Error("gatekeeper: failed to load service", "error", err, "leadId", e.LeadID, "serviceId", e.LeadServiceID)
				return
			}
			if domain.IsTerminal(service.Status, service.PipelineStage) {
				log.Info("gatekeeper: skipping terminal service on service added", "leadId", e.LeadID, "serviceId", e.LeadServiceID)
				return
			}

			if err := gatekeeper.Run(bg, e.LeadID, e.LeadServiceID, e.TenantID); err != nil {
				log.Error("gatekeeper run failed for new service", "error", err, "leadId", e.LeadID, "serviceId", e.LeadServiceID)
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

	eventBus.Subscribe(events.QuoteAccepted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.QuoteAccepted)
		if !ok {
			return nil
		}
		orchestrator.OnQuoteAccepted(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PartnerOfferRejected{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferRejected)
		if !ok {
			return nil
		}
		orchestrator.OnPartnerOfferRejected(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PartnerOfferAccepted{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferAccepted)
		if !ok {
			return nil
		}
		orchestrator.OnPartnerOfferAccepted(ctx, e)
		return nil
	}))

	eventBus.Subscribe(events.PartnerOfferExpired{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.PartnerOfferExpired)
		if !ok {
			return nil
		}
		orchestrator.OnPartnerOfferExpired(ctx, e)
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

func subscribeAttachmentUploaded(eventBus events.Bus, repo repository.LeadsRepository, batcher *photoAnalysisBatcher, log *logger.Logger) {
	eventBus.Subscribe(events.AttachmentUploaded{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.AttachmentUploaded)
		if !ok {
			return nil
		}

		// 1. Persist attachment record to database
		_, err := repo.CreateAttachment(ctx, repository.CreateAttachmentParams{
			LeadServiceID:  e.LeadServiceID,
			OrganizationID: e.TenantID,
			FileKey:        e.FileKey,
			FileName:       e.FileName,
			ContentType:    e.ContentType,
			SizeBytes:      e.SizeBytes,
			UploadedBy:     nil, // webhook uploads are system/anonymous
		})
		if err != nil {
			log.Error("failed to persist attachment record", "error", err, "leadServiceId", e.LeadServiceID)
			// Don't return error - continue with photo analysis
		}

		// 2. Trigger photo analysis for images
		if !isImageContentType(e.ContentType) {
			return nil
		}
		if batcher == nil {
			log.Warn("photo analysis batcher not configured")
			return nil
		}

		// Terminal check: don't analyze photos for terminal services
		service, err := repo.GetLeadServiceByID(ctx, e.LeadServiceID, e.TenantID)
		if err != nil {
			log.Error("photo batcher: failed to load service", "error", err, "serviceId", e.LeadServiceID)
			return nil
		}
		if domain.IsTerminal(service.Status, service.PipelineStage) {
			log.Info("photo batcher: skipping terminal service", "serviceId", e.LeadServiceID)
			return nil
		}

		batcher.OnImageUploaded(e.LeadID, e.LeadServiceID, e.TenantID)
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
	attachmentsHandler := handler.NewAttachmentsHandler(deps.Repo, deps.EventBus, deps.StorageSvc, deps.Config.GetMinioBucketLeadServiceAttachments(), deps.Validator)
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

// SetPublicViewers injects quote and appointment viewers for the public portal.
func (m *Module) SetPublicViewers(quoteViewer ports.QuotePublicViewer, apptViewer ports.AppointmentPublicViewer, slotViewer ports.AppointmentSlotProvider) {
	if m.publicHandler == nil {
		return
	}
	m.publicHandler.SetPublicViewers(quoteViewer, apptViewer, slotViewer)
}

// SetPublicOrgViewer injects organization contact info for the public portal.
func (m *Module) SetPublicOrgViewer(orgViewer ports.OrganizationPublicViewer) {
	if m.publicHandler == nil {
		return
	}
	m.publicHandler.SetPublicOrgViewer(orgViewer)
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

// SetCatalogReader sets the catalog reader on the Estimator agent.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetCatalogReader(cr ports.CatalogReader) {
	m.estimator.SetCatalogReader(cr)
	m.quoteGenerator.SetCatalogReader(cr)
}

// SetQuoteDrafter sets the quote drafter on the Estimator agent.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetQuoteDrafter(qd ports.QuoteDrafter) {
	m.estimator.SetQuoteDrafter(qd)
	m.quoteGenerator.SetQuoteDrafter(qd)
}

// SetPartnerOfferCreator sets the partner offer creator on the Dispatcher agent.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetPartnerOfferCreator(poc ports.PartnerOfferCreator) {
	m.dispatcher.SetOfferCreator(poc)
}

// GenerateQuoteFromPrompt runs the QuoteGenerator agent with a user prompt.
func (m *Module) GenerateQuoteFromPrompt(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) (*agent.GenerateResult, error) {
	return m.quoteGenerator.Generate(ctx, leadID, serviceID, tenantID, prompt, existingQuoteID)
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

	// Public lead portal routes (no auth middleware)
	publicGroup := ctx.V1.Group("/public/leads")
	m.publicHandler.RegisterRoutes(publicGroup)
}

// sseHandler returns the SSE handler with user ID extraction
func (m *Module) sseHandler() func(c *gin.Context) {
	return m.sse.Handler(
		func(c *gin.Context) (uuid.UUID, bool) {
			id := httpkit.GetIdentity(c)
			if !id.IsAuthenticated() {
				return uuid.UUID{}, false
			}
			return id.UserID(), true
		},
		func(c *gin.Context) (uuid.UUID, bool) {
			id := httpkit.GetIdentity(c)
			if !id.IsAuthenticated() {
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
