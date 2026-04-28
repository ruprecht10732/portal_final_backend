// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/qdrant"
	"portal_final_backend/platform/validator"
	adksession "portal_final_backend/platform/adk/session"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/adk/session"
)

// Module is the RAC_leads bounded context module implementing http.Module.
type Module struct {
	handler               *handler.Handler
	publicHandler         *handler.PublicHandler
	management            *management.Service
	notes                 *notes.Service
	gatekeeper            *agent.Gatekeeper
	estimator             agent.Estimator
	dispatcher            *agent.Dispatcher
	auditor               *agent.Auditor
	orchestrator          *Orchestrator
	callLogger            *agent.CallLogger
	quoteGenerator        agent.QuoteGenerator
	offerSummaryGenerator *agent.OfferSummaryGenerator
	replyAgent            *agent.ReplyAgent
	staleReEngagement     *maintenance.StaleLeadReEngagementService
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
	gatekeeperRunMu       sync.Mutex
	gatekeeperRunActive   map[uuid.UUID]bool
}

type AutomationScheduler interface {
	scheduler.GatekeeperScheduler
	scheduler.EstimatorScheduler
	scheduler.DispatcherScheduler
	scheduler.AuditorScheduler
}

type ModuleDeps struct {
	Config                *config.Config
	Log                   *logger.Logger
	OrchestratorLockRedis *redis.Client
	SessionRedis          *redis.Client
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
	if m.replyAgent != nil {
		m.replyAgent.SetOrganizationAISettingsReader(reader)
	}
	if m.staleReEngagement != nil {
		m.staleReEngagement.SetOrganizationAISettingsReader(reader)
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

	if deps.SessionRedis == nil {
		return nil, fmt.Errorf("leads module: SessionRedis is required")
	}
	sessionService := adksession.NewService(adksession.Config{
		Backend:     "redis",
		RedisClient: deps.SessionRedis,
		RedisPrefix: "adk:session:",
		RedisTTL:    24 * time.Hour,
	})

	callLogger, gatekeeper, estimator, dispatcher, auditor, quoteGenerator, offerSummaryGenerator, replyAgent, err := buildAgents(cfg, repo, storageSvc, scorer, eventBus, catalogReader, sessionService)
	if err != nil {
		return nil, err
	}
	if log != nil {
		log.Info("leads module: agents constructed successfully", "components", "call-logger,gatekeeper,calculator,matchmaker,auditor,offer-summary,whatsapp-reply,email-reply")
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
	h := buildHandlers(buildHandlersDeps{
		MgmtSvc:         mgmtSvc,
		NotesSvc:        notesSvc,
		Gatekeeper:      gatekeeper,
		CallLogger:      callLogger,
		SSEService:      sseService,
		EventBus:        eventBus,
		Repo:            repo,
		StorageSvc:      storageSvc,
		Config:          cfg,
		Validator:       val,
		CallLogQueue:    nil,
		GatekeeperQueue: nil,
	})
	publicHandler := handler.NewPublicHandler(repo, eventBus, sseService, storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), val)

	// Stale lead detector for the dashboard API
	staleDetector := maintenance.NewStaleLeadDetector(pool, log)
	h.SetStaleLeadDetector(staleDetector)

	// Stale lead AI-powered re-engagement suggestion generator
	staleReEngagementAgent := agent.NewStaleReEngagementAgent(resolveAgentModelConfig(cfg, config.LLMModelAgentStaleReEngagement, false), repo, sessionService)
	staleReEngagement := maintenance.NewStaleLeadReEngagementService(pool, staleReEngagementAgent, nil, log)
	h.SetStaleSuggester(staleReEngagement)

	module := &Module{
		handler:               h,
		publicHandler:         publicHandler,
		management:            mgmtSvc,
		notes:                 notesSvc,
		gatekeeper:            gatekeeper,
		estimator:             estimator,
		dispatcher:            dispatcher,
		auditor:               auditor,
		orchestrator:          orchestrator,
		callLogger:            callLogger,
		quoteGenerator:        quoteGenerator,
		offerSummaryGenerator: offerSummaryGenerator,
		replyAgent:            replyAgent,
		staleReEngagement:     staleReEngagement,
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
		ModelConfig:     resolveAgentModelConfig(cfg, config.LLMModelAgentQuoteGenerator, false),
		SessionService:  sessionService,
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

// StaleLeadReEngagement returns the AI re-engagement service for scheduler wiring.
func (m *Module) StaleLeadReEngagement() *maintenance.StaleLeadReEngagementService {
	if m == nil {
		return nil
	}
	return m.staleReEngagement
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
	if m.orchestrator == nil {
		return fmt.Errorf("leads module: orchestrator is not configured")
	}
	if m.log != nil && m.handler != nil && m.automationQueue != nil {
		m.log.Info("leads module: wiring verified", "automationQueue", true, "appointmentBooker", true, "leadUpdater", true)
	}
	return nil
}

func buildAgents(cfg *config.Config, repo repository.LeadsRepository, storageSvc storage.StorageService, scorer *scoring.Service, eventBus events.Bus, catalogReader ports.CatalogReader, sessionService session.Service) (*agent.CallLogger, *agent.Gatekeeper, agent.Estimator, *agent.Dispatcher, *agent.Auditor, agent.QuoteGenerator, *agent.OfferSummaryGenerator, *agent.ReplyAgent, error) {
	_ = storageSvc
	if err := validateAgentConfiguration(); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	callLogger, err := agent.NewCallLogger(resolveAgentModelConfig(cfg, config.LLMModelAgentCallLogger, true), repo, nil, eventBus, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	gatekeeper, err := agent.NewGatekeeper(
		agent.BuildLLM(resolveAgentModelConfig(cfg, config.LLMModelAgentGatekeeper, true)),
		repo, eventBus, scorer, sessionService,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	auditor, err := agent.NewAuditor(resolveAgentModelConfig(cfg, config.LLMModelAgentAuditor, true), repo, eventBus, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	aiClients := buildAIClients(cfg)

	estimator, err := agent.NewEstimatorAgent(agent.QuotingAgentConfig{
		ModelConfig:          resolveAgentModelConfig(cfg, config.LLMModelAgentEstimator, true),
		Repo:                 repo,
		EventBus:             eventBus,
		EmbeddingClient:      aiClients.embeddingClient,
		QdrantClient:         aiClients.qdrantClient,
		BouwmaatQdrantClient: aiClients.bouwmaatQdrantClient,
		CatalogQdrantClient:  aiClients.catalogQdrantClient,
		CatalogReader:        catalogReader,
	}, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	dispatcher, err := agent.NewDispatcher(resolveAgentModelConfig(cfg, config.LLMModelAgentDispatcher, true), repo, eventBus, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	quoteGenerator, err := agent.NewQuoteGeneratorAgent(agent.QuotingAgentConfig{
		ModelConfig:          resolveAgentModelConfig(cfg, config.LLMModelAgentQuoteGenerator, true),
		Repo:                 repo,
		EventBus:             eventBus,
		EmbeddingClient:      aiClients.embeddingClient,
		QdrantClient:         aiClients.qdrantClient,
		BouwmaatQdrantClient: aiClients.bouwmaatQdrantClient,
		CatalogQdrantClient:  aiClients.catalogQdrantClient,
		CatalogReader:        catalogReader,
	}, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	offerSummaryGenerator, err := agent.NewOfferSummaryGenerator(resolveAgentModelConfig(cfg, config.LLMModelAgentOfferSummaryGenerator, false), sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	replyAgent, err := buildReplyAgents(cfg, repo, sessionService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	return callLogger, gatekeeper, estimator, dispatcher, auditor, quoteGenerator, offerSummaryGenerator, replyAgent, nil
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

// resolveAgentModelConfig builds an openaicompat.Config for the given agent,
// combining the active provider preset with any per-agent model override.
func resolveAgentModelConfig(cfg *config.Config, agentName string, reasoning bool) openaicompat.Config {
	providerCfg, modelOverride := cfg.ResolveAgentModel(agentName)
	return agent.NewProviderModelConfig(providerCfg, reasoning, modelOverride)
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

func buildReplyAgents(cfg *config.Config, repo repository.LeadsRepository, sessionService session.Service) (*agent.ReplyAgent, error) {
	replyAgent, err := agent.NewReplyAgent("whatsapp", resolveAgentModelConfig(cfg, config.LLMModelAgentWhatsAppReply, false), repo, sessionService)
	if err != nil {
		return nil, err
	}

	return replyAgent, nil
}

// OfferSummaryGenerator exposes the AI summary generator for partner offers.
func (m *Module) OfferSummaryGenerator() ports.OfferSummaryGenerator {
	return m.offerSummaryGenerator
}

func (m *Module) WhatsAppReplyGenerator() ports.WhatsAppReplyGenerator {
	return m.replyAgent
}

func (m *Module) EmailReplyGenerator() ports.EmailReplyGenerator {
	return m.replyAgent
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

func subscribeOrchestrator(eventBus events.Bus, orchestrator *Orchestrator) {
	subscribeOrchestratorEvents(eventBus, orchestrator)
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

type buildHandlersDeps struct {
	MgmtSvc         *management.Service
	NotesSvc        *notes.Service
	Gatekeeper      *agent.Gatekeeper
	CallLogger      *agent.CallLogger
	SSEService      *sse.Service
	EventBus        events.Bus
	Repo            repository.LeadsRepository
	StorageSvc      storage.StorageService
	Config          *config.Config
	Validator       *validator.Validator
	CallLogQueue    scheduler.CallLogScheduler
	GatekeeperQueue scheduler.GatekeeperScheduler
}

func buildHandlers(deps buildHandlersDeps) *handler.Handler {
	return handler.New(handler.HandlerDeps{
		Mgmt:              deps.MgmtSvc,
		NotesSvc:          deps.NotesSvc,
		Gatekeeper:        deps.Gatekeeper,
		CallLogger:        deps.CallLogger,
		SSE:               deps.SSEService,
		EventBus:          deps.EventBus,
		Repo:              deps.Repo,
		Validator:         deps.Validator,
		CallLogQueue:      deps.CallLogQueue,
		GatekeeperQueue:   deps.GatekeeperQueue,
		Storage:           deps.StorageSvc,
		AttachmentsBucket: deps.Config.GetMinioBucketLeadServiceAttachments(),
	})
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

	// Prevent concurrent gatekeeper runs on the same service. The asynq
	// uniqueness window (45s) can expire while a slow LLM call is still in
	// progress, allowing a second task to start. This mutex ensures only
	// one run executes per service at a time.
	if !m.tryAcquireGatekeeperRun(serviceID) {
		if m.log != nil {
			m.log.Info("gatekeeper: skipping concurrent run for service", "serviceId", serviceID, "leadId", leadID)
		}
		return nil
	}
	defer m.releaseGatekeeperRun(serviceID)

	if err := m.gatekeeper.Run(ctx, leadID, serviceID, tenantID); err != nil {
		// Record abort in deduper so the next data-change trigger respects a
		// cooldown period instead of immediately re-enqueuing.
		if m.gatekeeperDeduper != nil && strings.Contains(err.Error(), "tool call limit exceeded") {
			m.gatekeeperDeduper.RecordAbort(serviceID)
			if m.log != nil {
				m.log.Warn("gatekeeper: run aborted (budget exceeded), cooldown applied", "serviceId", serviceID, "leadId", leadID)
			}
		}
		return err
	}
	if m.orchestrator != nil {
		m.orchestrator.maybeAutoDisqualifyJunk(ctx, leadID, serviceID, tenantID)
	}
	return nil
}

func (m *Module) tryAcquireGatekeeperRun(serviceID uuid.UUID) bool {
	m.gatekeeperRunMu.Lock()
	defer m.gatekeeperRunMu.Unlock()
	if m.gatekeeperRunActive == nil {
		m.gatekeeperRunActive = make(map[uuid.UUID]bool)
	}
	if m.gatekeeperRunActive[serviceID] {
		return false
	}
	m.gatekeeperRunActive[serviceID] = true
	return true
}

func (m *Module) releaseGatekeeperRun(serviceID uuid.UUID) {
	m.gatekeeperRunMu.Lock()
	defer m.gatekeeperRunMu.Unlock()
	delete(m.gatekeeperRunActive, serviceID)
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
	if m.replyAgent != nil {
		m.replyAgent.SetContextReaders(quoteReader, apptViewer, userReader)
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
