// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/adapters/storage"
	catalogrepo "portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/adapters"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/maintenance"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/notes"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/scoring"
	"portal_final_backend/internal/maps"
	notificationoutbox "portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/orchestration"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/qdrant"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
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
	estimator             agent.Estimator
	dispatcher            *agent.Dispatcher
	auditor               *agent.Auditor
	orchestrator          *Orchestrator
	photoAnalyzer         *agent.PhotoAnalyzer
	callLogger            *agent.CallLogger
	quoteGenerator        agent.QuoteGenerator
	offerSummaryGenerator *agent.OfferSummaryGenerator
	whatsAppReplyAgent    *agent.WhatsAppReplyAgent
	emailReplyAgent       *agent.EmailReplyAgent
	subsidyAnalyzerSvc    *SubsidyAnalyzerService
	sse                   *sse.Service
	eventBus              events.Bus
	repo                  repository.LeadsRepository
	storage               storage.StorageService
	attachmentsBucket     string
	log                   *logger.Logger
	scorer                *scoring.Service
	automationQueue       AutomationScheduler
	gatekeeperDeduper     gatekeeperTriggerDeduper
	estimatorDeduper      triggerFingerprintDeduper
	dispatcherDeduper     triggerFingerprintDeduper
}

type AutomationScheduler interface {
	scheduler.GatekeeperScheduler
	scheduler.EstimatorScheduler
	scheduler.DispatcherScheduler
	scheduler.PhotoAnalysisScheduler
	scheduler.AuditorScheduler
}

type ModuleDeps struct {
	Config                *config.Config
	Log                   *logger.Logger
	OrchestratorLockRedis *redis.Client
}

// SetOrganizationAISettingsReader injects a tenant-scoped settings reader into
// the orchestrator and lead agents that need to respect org-level automation
// toggles and catalog gap heuristics.
func (m *Module) SetOrganizationAISettingsReader(reader ports.OrganizationAISettingsReader) {
	if m == nil {
		return
	}
	if m.orchestrator != nil {
		m.orchestrator.SetOrganizationAISettingsReader(reader)
	}
	if m.gatekeeper != nil {
		m.gatekeeper.SetOrganizationAISettingsReader(reader)
	}
	if m.estimator != nil {
		m.estimator.SetOrganizationAISettingsReader(reader)
	}
	if m.dispatcher != nil {
		m.dispatcher.SetOrganizationAISettingsReader(reader)
	}
	if m.quoteGenerator != nil {
		m.quoteGenerator.SetOrganizationAISettingsReader(reader)
	}
	if m.whatsAppReplyAgent != nil {
		m.whatsAppReplyAgent.SetOrganizationAISettingsReader(reader)
	}
	if m.emailReplyAgent != nil {
		m.emailReplyAgent.SetOrganizationAISettingsReader(reader)
	}
	if m.photoAnalysisHandler != nil {
		m.photoAnalysisHandler.SetOrganizationAISettingsReader(reader)
	}
}

// NewModule creates and initializes the RAC_leads module with all its dependencies.
func NewModule(ctx context.Context, pool *pgxpool.Pool, eventBus events.Bus, storageSvc storage.StorageService, val *validator.Validator, deps ModuleDeps) (*Module, error) {
	cfg := deps.Config
	log := deps.Log
	orchestratorLockRedis := deps.OrchestratorLockRedis

	// Create shared repository
	repo := repository.New(pool)

	// Leads agents need to safely hydrate catalog items from DB.
	// Draft products are excluded by the adapter.
	catalogRepo := catalogrepo.New(pool)
	catalogReader := adapters.NewCatalogReaderAdapter(catalogRepo)

	// Score service for lead scoring
	scorer := scoring.New(repo, log)

	photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, auditor, quoteGenerator, offerSummaryGenerator, whatsAppReplyAgent, emailReplyAgent, err := buildAgents(cfg, repo, storageSvc, scorer, eventBus, catalogReader)
	if err != nil {
		return nil, err
	}
	if log != nil {
		log.Info("leads module: agents constructed successfully", "components", "photo-analyzer,call-logger,gatekeeper,calculator,matchmaker,auditor,offer-summary,whatsapp-reply,email-reply")
	}

	// SSE service for real-time notifications
	sseService := sse.New()

	// Create focused services (vertical slices)
	mapsSvc := maps.NewService(log)
	mgmtSvc := management.New(repo, eventBus, mapsSvc)
	mgmtSvc.SetLeadScorer(scorer)
	notesSvc := notes.New(repo)
	callLogger.SetLeadUpdater(mgmtSvc)

	// Create orchestrator and event listeners
	outboxRepo := notificationoutbox.New(pool)
	runLocker := newOrchestratorRunLocker(orchestratorLockRedis, log)
	gatekeeperDeduper := newGatekeeperTriggerDeduper(orchestratorLockRedis, gatekeeperTriggerFingerprintTTL, log)
	estimatorDeduper := newTriggerFingerprintDeduper(orchestratorLockRedis, gatekeeperTriggerFingerprintTTL, estimatorTriggerFingerprintPrefix, log)
	dispatcherDeduper := newTriggerFingerprintDeduper(orchestratorLockRedis, gatekeeperTriggerFingerprintTTL, dispatcherTriggerFingerprintPrefix, log)
	orchestrator := NewOrchestrator(OrchestratorAgents{
		Gatekeeper: gatekeeper,
		Estimator:  estimator,
		Dispatcher: dispatcher,
		Auditor:    auditor,
	}, repo, outboxRepo, eventBus, sseService, log, runLocker)
	orchestrator.gatekeeperDeduper = gatekeeperDeduper
	orchestrator.estimatorDeduper = estimatorDeduper
	orchestrator.dispatcherDeduper = dispatcherDeduper
	orchestrator.SetReconciliationEnabled(cfg.IsLeadsReconciliationEnabled())
	if ctx == nil {
		ctx = context.Background()
	}
	orchestrator.StartCleanupLoop(ctx)
	subscribeOrchestrator(eventBus, orchestrator)

	// Create handlers
	h, attachmentsHandler, photoAnalysisHandler := buildHandlers(buildHandlersDeps{
		MgmtSvc:            mgmtSvc,
		NotesSvc:           notesSvc,
		Gatekeeper:         gatekeeper,
		CallLogger:         callLogger,
		SSEService:         sseService,
		EventBus:           eventBus,
		Repo:               repo,
		StorageSvc:         storageSvc,
		Config:             cfg,
		Validator:          val,
		PhotoAnalyzer:      photoAnalyzer,
		CallLogQueue:       nil,
		GatekeeperQueue:    nil,
		PhotoAnalysisQueue: nil,
	})
	publicHandler := handler.NewPublicHandler(repo, eventBus, sseService, storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), val)

	// Stale lead detector for the dashboard API
	staleDetector := maintenance.NewStaleLeadDetector(pool, log)
	h.SetStaleLeadDetector(staleDetector)

	module := &Module{
		handler:               h,
		attachmentsHandler:    attachmentsHandler,
		photoAnalysisHandler:  photoAnalysisHandler,
		publicHandler:         publicHandler,
		management:            mgmtSvc,
		notes:                 notesSvc,
		gatekeeper:            gatekeeper,
		estimator:             estimator,
		dispatcher:            dispatcher,
		auditor:               auditor,
		orchestrator:          orchestrator,
		photoAnalyzer:         photoAnalyzer,
		callLogger:            callLogger,
		quoteGenerator:        quoteGenerator,
		offerSummaryGenerator: offerSummaryGenerator,
		whatsAppReplyAgent:    whatsAppReplyAgent,
		emailReplyAgent:       emailReplyAgent,
		subsidyAnalyzerSvc:    nil, // Will be set after instantiation
		sse:                   sseService,
		eventBus:              eventBus,
		repo:                  repo,
		storage:               storageSvc,
		attachmentsBucket:     cfg.GetMinioBucketLeadServiceAttachments(),
		log:                   log,
		scorer:                scorer,
		gatekeeperDeduper:     gatekeeperDeduper,
		estimatorDeduper:      estimatorDeduper,
		dispatcherDeduper:     dispatcherDeduper,
	}

	// Subsidy analyzer service (no ADK agent instantiation here; done lazily when needed)
	subsidyAnalyzerSvc := NewSubsidyAnalyzerService(SubsidyAnalyzerServiceConfig{
		Repo:            repo,
		QuoteRepo:       nil, // Will be injected from quotes module
		EventBus:        eventBus,
		SSEService:      sseService,
		SchedulerClient: nil, // Will be injected from main.go/quotes module
		Log:             log,
		MoonshotAPIKey:  cfg.MoonshotAPIKey,
		LLMModel:        cfg.ResolveLLMModel(config.LLMModelAgentQuoteGenerator),
	})
	module.subsidyAnalyzerSvc = subsidyAnalyzerSvc

	subscribeLeadCreated(eventBus, repo, module, log)
	subscribeLeadServiceAdded(eventBus, repo, module, log)
	subscribeAttachmentUploaded(eventBus, repo, module, log)
	if log != nil {
		log.Info("leads module: event subscriptions registered", "subscriptions", "lead-created,lead-service-added,attachment-uploaded,orchestrator")
	}

	return module, nil
}

// GetSubsidyAnalyzerService returns the subsidy analyzer service for external wiring.
func (m *Module) GetSubsidyAnalyzerService() *SubsidyAnalyzerService {
	if m == nil {
		return nil
	}
	return m.subsidyAnalyzerSvc
}

func (m *Module) VerifyWiring() error {
	if m == nil {
		return fmt.Errorf("leads module: module is nil")
	}
	if m.callLogger == nil {
		return fmt.Errorf("leads module: call logger is not configured")
	}
	if !m.callLogger.HasLeadUpdater() {
		return fmt.Errorf("leads module: call logger lead updater is not configured")
	}
	if !m.callLogger.HasAppointmentBooker() {
		return fmt.Errorf("leads module: appointment booker is not configured")
	}
	if m.automationQueue == nil {
		return fmt.Errorf("leads module: automation scheduler is not configured")
	}
	if m.handler == nil {
		return fmt.Errorf("leads module: handler is not configured")
	}
	if m.photoAnalysisHandler == nil {
		return fmt.Errorf("leads module: photo analysis handler is not configured")
	}
	if m.orchestrator == nil {
		return fmt.Errorf("leads module: orchestrator is not configured")
	}
	if m.log != nil && m.handler != nil && m.automationQueue != nil {
		m.log.Info("leads module: wiring verified", "automationQueue", true, "appointmentBooker", true, "leadUpdater", true)
	}
	return nil
}

func buildAgents(cfg *config.Config, repo repository.LeadsRepository, storageSvc storage.StorageService, scorer *scoring.Service, eventBus events.Bus, catalogReader ports.CatalogReader) (*agent.PhotoAnalyzer, *agent.CallLogger, *agent.Gatekeeper, agent.Estimator, *agent.Dispatcher, *agent.Auditor, agent.QuoteGenerator, *agent.OfferSummaryGenerator, *agent.WhatsAppReplyAgent, *agent.EmailReplyAgent, error) {
	_ = storageSvc
	if err := validateAgentConfiguration(); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	photoAnalyzer, err := agent.NewPhotoAnalyzer(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentPhotoAnalyzer), repo)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	callLogger, err := agent.NewCallLogger(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentCallLogger), repo, nil, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	gatekeeper, err := agent.NewGatekeeper(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentGatekeeper), repo, eventBus, scorer)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	auditor, err := agent.NewAuditor(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentAuditor), repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	aiClients := buildAIClients(cfg)

	estimator, err := agent.NewEstimatorAgent(agent.QuotingAgentConfig{
		APIKey:               cfg.MoonshotAPIKey,
		Model:                cfg.ResolveLLMModel(config.LLMModelAgentEstimator),
		Repo:                 repo,
		EventBus:             eventBus,
		EmbeddingClient:      aiClients.embeddingClient,
		QdrantClient:         aiClients.qdrantClient,
		BouwmaatQdrantClient: aiClients.bouwmaatQdrantClient,
		CatalogQdrantClient:  aiClients.catalogQdrantClient,
		CatalogReader:        catalogReader,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	dispatcher, err := agent.NewDispatcher(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentDispatcher), repo, eventBus)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	quoteGenerator, err := agent.NewQuoteGeneratorAgent(agent.QuotingAgentConfig{
		APIKey:               cfg.MoonshotAPIKey,
		Model:                cfg.ResolveLLMModel(config.LLMModelAgentQuoteGenerator),
		Repo:                 repo,
		EventBus:             eventBus,
		EmbeddingClient:      aiClients.embeddingClient,
		QdrantClient:         aiClients.qdrantClient,
		BouwmaatQdrantClient: aiClients.bouwmaatQdrantClient,
		CatalogQdrantClient:  aiClients.catalogQdrantClient,
		CatalogReader:        catalogReader,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	offerSummaryGenerator, err := agent.NewOfferSummaryGenerator(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentOfferSummaryGenerator))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	whatsAppReplyAgent, emailReplyAgent, err := buildReplyAgents(cfg, repo)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	return photoAnalyzer, callLogger, gatekeeper, estimator, dispatcher, auditor, quoteGenerator, offerSummaryGenerator, whatsAppReplyAgent, emailReplyAgent, nil
}

type aiClients struct {
	embeddingClient      *embeddings.Client
	qdrantClient         *qdrant.Client
	catalogQdrantClient  *qdrant.Client
	bouwmaatQdrantClient *qdrant.Client
}

func validateAgentConfiguration() error {
	if err := orchestration.ValidateAgentWorkspaces(); err != nil {
		return err
	}
	return agent.ValidatePromptTemplates()
}

func buildAIClients(cfg *config.Config) aiClients {
	return aiClients{
		embeddingClient:      buildEmbeddingClient(cfg),
		qdrantClient:         buildQdrantClient(cfg),
		catalogQdrantClient:  buildScopedQdrantClient(cfg, cfg.GetCatalogEmbeddingCollection()),
		bouwmaatQdrantClient: buildScopedQdrantClient(cfg, cfg.GetBouwmaatEmbeddingCollection()),
	}
}

func buildEmbeddingClient(cfg *config.Config) *embeddings.Client {
	if !cfg.IsEmbeddingEnabled() {
		return nil
	}
	return embeddings.NewClient(embeddings.Config{
		BaseURL: cfg.GetEmbeddingAPIURL(),
		APIKey:  cfg.GetEmbeddingAPIKey(),
	})
}

func buildQdrantClient(cfg *config.Config) *qdrant.Client {
	if !cfg.IsQdrantEnabled() {
		return nil
	}
	return qdrant.NewClient(qdrant.Config{
		BaseURL:    cfg.GetQdrantURL(),
		APIKey:     cfg.GetQdrantAPIKey(),
		Collection: cfg.GetQdrantCollection(),
	})
}

func buildScopedQdrantClient(cfg *config.Config, collection string) *qdrant.Client {
	if cfg.GetQdrantURL() == "" || collection == "" {
		return nil
	}
	return qdrant.NewClient(qdrant.Config{
		BaseURL:    cfg.GetQdrantURL(),
		APIKey:     cfg.GetQdrantAPIKey(),
		Collection: collection,
	})
}

func buildReplyAgents(cfg *config.Config, repo repository.LeadsRepository) (*agent.WhatsAppReplyAgent, *agent.EmailReplyAgent, error) {
	whatsAppReplyAgent, err := agent.NewWhatsAppReplyAgent(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentWhatsAppReply), repo)
	if err != nil {
		return nil, nil, err
	}

	emailReplyAgent, err := agent.NewEmailReplyAgent(cfg.MoonshotAPIKey, cfg.ResolveLLMModel(config.LLMModelAgentWhatsAppReply), repo)
	if err != nil {
		return nil, nil, err
	}

	return whatsAppReplyAgent, emailReplyAgent, nil
}

// OfferSummaryGenerator exposes the AI summary generator for partner offers.
func (m *Module) OfferSummaryGenerator() ports.OfferSummaryGenerator {
	return m.offerSummaryGenerator
}

func (m *Module) WhatsAppReplyGenerator() ports.WhatsAppReplyGenerator {
	return m.whatsAppReplyAgent
}

func (m *Module) EmailReplyGenerator() ports.EmailReplyGenerator {
	return m.emailReplyAgent
}

func subscribeLeadCreated(eventBus events.Bus, repo repository.LeadsRepository, module *Module, log *logger.Logger) {
	eventBus.Subscribe(events.LeadCreated{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadCreated)
		if !ok {
			return nil
		}

		runInitialGatekeeper(context.Background(), repo, module, log, initialGatekeeperTrigger{
			LeadID:    e.LeadID,
			ServiceID: e.LeadServiceID,
			TenantID:  e.TenantID,
			Source:    "lead created",
		})

		return nil
	}))
}

func subscribeLeadServiceAdded(eventBus events.Bus, repo repository.LeadsRepository, module *Module, log *logger.Logger) {
	eventBus.Subscribe(events.LeadServiceAdded{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.LeadServiceAdded)
		if !ok {
			return nil
		}

		runInitialGatekeeper(context.Background(), repo, module, log, initialGatekeeperTrigger{
			LeadID:    e.LeadID,
			ServiceID: e.LeadServiceID,
			TenantID:  e.TenantID,
			Source:    "service added",
		})

		return nil
	}))
}

type initialGatekeeperTrigger struct {
	LeadID    uuid.UUID
	ServiceID uuid.UUID
	TenantID  uuid.UUID
	Source    string
}

func runInitialGatekeeper(ctx context.Context, repo repository.LeadsRepository, module *Module, log *logger.Logger, trigger initialGatekeeperTrigger) {
	if shouldSkipInitialGatekeeper(ctx, repo, log, trigger) {
		return
	}
	if enqueueGatekeeperRun(ctx, repo, module, log, trigger) {
		return
	}
	if log != nil {
		log.Error("gatekeeper queue not configured for initial run", "leadId", trigger.LeadID, "serviceId", trigger.ServiceID, "source", trigger.Source)
	}
}

func shouldSkipInitialGatekeeper(ctx context.Context, repo repository.LeadsRepository, log *logger.Logger, trigger initialGatekeeperTrigger) bool {
	service, err := repo.GetLeadServiceByID(ctx, trigger.ServiceID, trigger.TenantID)
	if err != nil {
		log.Error("gatekeeper: failed to load service", "error", err, "leadId", trigger.LeadID, "serviceId", trigger.ServiceID)
		return true
	}
	if domain.IsTerminal(service.Status, service.PipelineStage) {
		log.Info("gatekeeper: skipping terminal service", "leadId", trigger.LeadID, "serviceId", trigger.ServiceID, "source", trigger.Source)
		return true
	}
	if !domain.AllowsGatekeeperEvaluation(service.PipelineStage) {
		log.Info("gatekeeper: skipping initial run for unsupported stage", "leadId", trigger.LeadID, "serviceId", trigger.ServiceID, "stage", service.PipelineStage, "source", trigger.Source)
		return true
	}
	if attachments, attErr := repo.ListAttachmentsByService(ctx, trigger.ServiceID, trigger.TenantID); attErr == nil && hasImageAttachments(attachments) {
		log.Info("gatekeeper: deferring initial run until photo analysis concludes", "leadId", trigger.LeadID, "serviceId", trigger.ServiceID, "source", trigger.Source)
		return true
	}
	return false
}

func enqueueGatekeeperRun(ctx context.Context, repo repository.LeadsRepository, module *Module, log *logger.Logger, trigger initialGatekeeperTrigger) bool {
	if module == nil {
		return false
	}
	return maybeEnqueueGatekeeperRun(gatekeeperEnqueueRequest{
		ctx:       ctx,
		repo:      repo,
		deduper:   module.gatekeeperDeduper,
		queue:     module.automationQueue,
		log:       log,
		leadID:    trigger.LeadID,
		serviceID: trigger.ServiceID,
		tenantID:  trigger.TenantID,
		source:    trigger.Source,
	})
}

func hasImageAttachments(items []repository.Attachment) bool {
	for _, att := range items {
		if att.ContentType != nil && isImageContentType(*att.ContentType) {
			return true
		}
	}
	return false
}

func subscribeOrchestrator(eventBus events.Bus, orchestrator *Orchestrator) {
	agentCoordinator := newAgentCoordinator(orchestrator)
	stateReconciler := newStateReconciler(orchestrator)
	pipelineManager := newPipelineManager(orchestrator)

	subscribeAgentCoordinator(eventBus, agentCoordinator)
	subscribeStateReconciler(eventBus, stateReconciler)
	subscribePipelineManager(eventBus, pipelineManager)
}

func subscribeAttachmentUploaded(eventBus events.Bus, repo repository.LeadsRepository, module *Module, log *logger.Logger) {
	eventBus.Subscribe(events.AttachmentUploaded{}.EventName(), events.HandlerFunc(func(ctx context.Context, event events.Event) error {
		e, ok := event.(events.AttachmentUploaded)
		if !ok {
			return nil
		}

		if shouldContinue := ensureAttachmentUploadedRecord(ctx, repo, log, e); !shouldContinue {
			return nil
		}

		// 2. Trigger photo analysis for images
		if !isImageContentType(e.ContentType) {
			return nil
		}
		if module == nil || module.photoAnalysisHandler == nil {
			log.Warn("photo analysis handler not configured")
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

		queueOrRunPhotoAnalysis(ctx, module, log, e.LeadID, e.LeadServiceID, e.TenantID)
		return nil
	}))
}

func ensureAttachmentUploadedRecord(ctx context.Context, repo repository.LeadsRepository, log *logger.Logger, e events.AttachmentUploaded) bool {
	if attachmentRecordExists(ctx, repo, log, e) {
		return true
	}
	if strings.TrimSpace(e.FileKey) == "" {
		log.Warn("attachment uploaded event missing file key; skipping persistence", "attachmentId", e.AttachmentID, "leadServiceId", e.LeadServiceID, "fileName", e.FileName)
		return false
	}
	if err := createAttachmentFromEvent(ctx, repo, e); err != nil {
		log.Error("failed to persist attachment record", "error", err, "leadServiceId", e.LeadServiceID, "fileKey", e.FileKey)
	}
	return true
}

func attachmentRecordExists(ctx context.Context, repo repository.LeadsRepository, log *logger.Logger, e events.AttachmentUploaded) bool {
	if e.AttachmentID == uuid.Nil {
		return false
	}
	_, err := repo.GetAttachmentByID(ctx, e.AttachmentID, e.TenantID)
	if err == nil {
		return true
	}
	if !errors.Is(err, repository.ErrAttachmentNotFound) {
		log.Error("failed to load attachment record", "error", err, "attachmentId", e.AttachmentID, "leadServiceId", e.LeadServiceID)
	}
	return false
}

func createAttachmentFromEvent(ctx context.Context, repo repository.LeadsRepository, e events.AttachmentUploaded) error {
	_, err := repo.CreateAttachment(ctx, repository.CreateAttachmentParams{
		LeadServiceID:  e.LeadServiceID,
		OrganizationID: e.TenantID,
		FileKey:        e.FileKey,
		FileName:       e.FileName,
		ContentType:    e.ContentType,
		SizeBytes:      e.SizeBytes,
		UploadedBy:     nil, // webhook uploads are system/anonymous
	})
	return err
}

func queueOrRunPhotoAnalysis(ctx context.Context, module *Module, log *logger.Logger, leadID, serviceID, tenantID uuid.UUID) {
	if module != nil && module.automationQueue != nil {
		if err := module.automationQueue.EnqueuePhotoAnalysisIn(ctx, scheduler.PhotoAnalysisPayload{
			TenantID:      tenantID.String(),
			LeadID:        leadID.String(),
			LeadServiceID: serviceID.String(),
		}, 30*time.Second); err != nil {
			log.Error("photo analysis queue enqueue failed", "error", err, "leadId", leadID, "serviceId", serviceID)
		}
		return
	}
	if log != nil {
		log.Error("photo analysis queue not configured", "leadId", leadID, "serviceId", serviceID)
	}
}

type buildHandlersDeps struct {
	MgmtSvc            *management.Service
	NotesSvc           *notes.Service
	Gatekeeper         *agent.Gatekeeper
	CallLogger         *agent.CallLogger
	SSEService         *sse.Service
	EventBus           events.Bus
	Repo               repository.LeadsRepository
	StorageSvc         storage.StorageService
	Config             *config.Config
	Validator          *validator.Validator
	PhotoAnalyzer      *agent.PhotoAnalyzer
	CallLogQueue       scheduler.CallLogScheduler
	GatekeeperQueue    scheduler.GatekeeperScheduler
	PhotoAnalysisQueue scheduler.PhotoAnalysisScheduler
}

func buildHandlers(deps buildHandlersDeps) (*handler.Handler, *handler.AttachmentsHandler, *handler.PhotoAnalysisHandler) {
	notesHandler := handler.NewNotesHandler(deps.NotesSvc, deps.Repo, deps.EventBus, deps.Validator)
	attachmentsHandler := handler.NewAttachmentsHandler(deps.Repo, deps.EventBus, deps.StorageSvc, deps.Config.GetMinioBucketLeadServiceAttachments(), deps.Validator)
	photoAnalysisHandler := handler.NewPhotoAnalysisHandler(deps.PhotoAnalyzer, deps.Repo, deps.StorageSvc, deps.Config.GetMinioBucketLeadServiceAttachments(), deps.SSEService, deps.Validator, deps.EventBus)
	photoAnalysisHandler.SetPhotoAnalysisScheduler(deps.PhotoAnalysisQueue)
	h := handler.New(handler.HandlerDeps{
		Mgmt:            deps.MgmtSvc,
		NotesHandler:    notesHandler,
		Gatekeeper:      deps.Gatekeeper,
		CallLogger:      deps.CallLogger,
		SSE:             deps.SSEService,
		EventBus:        deps.EventBus,
		Repo:            deps.Repo,
		Validator:       deps.Validator,
		CallLogQueue:    deps.CallLogQueue,
		GatekeeperQueue: deps.GatekeeperQueue,
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
	if m == nil {
		return
	}
	if booker == nil {
		if m.log != nil {
			m.log.Error("leads module: SetAppointmentBooker called with nil booker")
		}
		panic("leads module: appointment booker is required")
	}
	m.callLogger.SetAppointmentBooker(booker)
}

// SetCallLogScheduler injects the scheduler-backed queue for async call logging.
func (m *Module) SetCallLogScheduler(queue scheduler.CallLogScheduler) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.SetCallLogScheduler(queue)
}

func (m *Module) SetAutomationScheduler(queue AutomationScheduler) {
	if m == nil {
		return
	}
	if queue == nil {
		if m.log != nil {
			m.log.Error("leads module: SetAutomationScheduler called with nil queue")
		}
		panic("leads module: automation scheduler is required")
	}
	m.automationQueue = queue
	if m.handler != nil {
		m.handler.SetGatekeeperScheduler(queue)
	}
	if m.photoAnalysisHandler != nil {
		m.photoAnalysisHandler.SetPhotoAnalysisScheduler(queue)
	}
	if m.orchestrator != nil {
		m.orchestrator.SetAutomationScheduler(queue)
	}
}

// ProcessLogCallJob executes a queued call log summary and publishes lead data updates.
func (m *Module) ProcessLogCallJob(ctx context.Context, leadID, serviceID, userID, tenantID uuid.UUID, summary string) error {
	if m == nil || m.callLogger == nil {
		return nil
	}

	if _, err := m.callLogger.ProcessSummary(ctx, leadID, serviceID, userID, tenantID, summary); err != nil {
		return err
	}

	if m.eventBus != nil {
		m.eventBus.Publish(ctx, events.LeadDataChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        leadID,
			LeadServiceID: serviceID,
			TenantID:      tenantID,
			Source:        "call_log",
		})
	}

	return nil
}

func (m *Module) ProcessGatekeeperRun(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	if m == nil || m.gatekeeper == nil {
		return nil
	}
	if err := m.gatekeeper.Run(ctx, leadID, serviceID, tenantID); err != nil {
		return err
	}
	if m.orchestrator != nil {
		m.orchestrator.maybeAutoDisqualifyJunk(ctx, leadID, serviceID, tenantID)
	}
	return nil
}

func (m *Module) ProcessEstimatorRun(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, force bool) error {
	if m == nil || m.estimator == nil {
		return nil
	}
	return m.estimator.Execute(ctx, leadID, serviceID, tenantID, force)
}

func (m *Module) ProcessDispatcherRun(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	if m == nil || m.dispatcher == nil {
		return nil
	}
	err := m.dispatcher.Run(ctx, leadID, serviceID, tenantID)
	if err != nil && m.orchestrator != nil {
		m.orchestrator.recordDispatcherFailure(context.Background(), leadID, serviceID, tenantID)
	}
	return err
}

func (m *Module) ProcessPhotoAnalysisJob(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userID *uuid.UUID, contextInfo string) error {
	if m == nil || m.photoAnalysisHandler == nil {
		return nil
	}
	return m.photoAnalysisHandler.ProcessPhotoAnalysisJob(ctx, leadID, serviceID, tenantID, userID, contextInfo)
}

func (m *Module) ProcessAuditVisitReportJob(ctx context.Context, leadID, serviceID, tenantID, appointmentID uuid.UUID) error {
	if m == nil || m.auditor == nil {
		return nil
	}
	return m.auditor.AuditVisitReport(ctx, leadID, serviceID, tenantID, appointmentID)
}

func (m *Module) ProcessAuditCallLogJob(ctx context.Context, leadID, serviceID, tenantID uuid.UUID) error {
	if m == nil || m.auditor == nil {
		return nil
	}
	return m.auditor.AuditCallLog(ctx, leadID, serviceID, tenantID)
}

// SetPublicViewers injects quote and appointment viewers for the public portal.
func (m *Module) SetPublicViewers(quoteViewer ports.QuotePublicViewer, apptViewer ports.AppointmentPublicViewer, slotViewer ports.AppointmentSlotProvider) {
	if m.publicHandler == nil {
		return
	}
	m.publicHandler.SetPublicViewers(quoteViewer, apptViewer, slotViewer)
}

func (m *Module) SetReplyContextReaders(quoteReader ports.ReplyQuoteReader, apptViewer ports.AppointmentPublicViewer, userReader ports.ReplyUserReader) {
	if m == nil {
		return
	}
	if m.whatsAppReplyAgent != nil {
		m.whatsAppReplyAgent.SetContextReaders(quoteReader, apptViewer, userReader)
	}
	if m.emailReplyAgent != nil {
		m.emailReplyAgent.SetContextReaders(quoteReader, apptViewer, userReader)
	}
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

// SetPricingIntelligenceReader injects read-only quote pricing intelligence into estimator agents.
func (m *Module) SetPricingIntelligenceReader(reader ports.PricingIntelligenceReader) {
	m.estimator.SetPricingIntelligenceReader(reader)
	m.quoteGenerator.SetPricingIntelligenceReader(reader)
}

// SetPartnerOfferCreator sets the partner offer creator on the Dispatcher agent.
// This is called after module initialization to break circular dependencies.
func (m *Module) SetPartnerOfferCreator(poc ports.PartnerOfferCreator) {
	m.dispatcher.SetOfferCreator(poc)
}

// QuoteGeneratorAgent exposes the prompt-driven quote generator through its narrow interface.
func (m *Module) QuoteGeneratorAgent() agent.QuoteGenerator {
	if m == nil {
		return nil
	}
	return m.quoteGenerator
}

// RegisterRoutes mounts RAC_leads routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// All RAC_leads routes require authentication
	leadsGroup := ctx.Protected.Group("/leads")
	m.handler.RegisterRoutes(leadsGroup)
	adminLeadsGroup := ctx.Admin.Group("/leads")
	m.handler.RegisterAdminRoutes(adminLeadsGroup)

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
