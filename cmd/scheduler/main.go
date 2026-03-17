package main

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/appointments"
	"portal_final_backend/internal/catalog"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/imap"
	"portal_final_backend/internal/leads"
	"portal_final_backend/internal/leads/maintenance"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/partners"
	"portal_final_backend/internal/quotes"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/tasks"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/internal/whatsappagent"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/ai/transcription"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/rediskit"
	"portal_final_backend/platform/validator"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const schedulerStorageBucketEnsureErrPrefix = "failed to ensure storage bucket exists: "
const schedulerStorageBucketEnsureErrMsg = "failed to ensure storage bucket exists"

func noOpRedisCloser() {
	// Intentionally empty: used when no Redis client was initialized.
}

func ensureBucket(ctx context.Context, log *logger.Logger, storageSvc storage.StorageService, name, bucket string) {
	if err := withRetry(ctx, log, "ensure "+name+" bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, bucket)
	}); err != nil {
		log.Error(schedulerStorageBucketEnsureErrMsg, "error", err, "bucket", bucket)
		panic(schedulerStorageBucketEnsureErrPrefix + err.Error())
	}
}

func initStorageOrPanic(ctx context.Context, cfg *config.Config, log *logger.Logger) storage.StorageService {
	storageSvc, err := storage.NewMinIOService(cfg)
	if err != nil {
		log.Error("failed to initialize storage service", "error", err)
		panic("failed to initialize storage service: " + err.Error())
	}

	ensureBucket(ctx, log, storageSvc, "lead-service-attachments", cfg.GetMinioBucketLeadServiceAttachments())
	ensureBucket(ctx, log, storageSvc, "catalog-assets", cfg.GetMinioBucketCatalogAssets())
	ensureBucket(ctx, log, storageSvc, "partner-logos", cfg.GetMinioBucketPartnerLogos())
	ensureBucket(ctx, log, storageSvc, "organization-logos", cfg.GetMinioBucketOrganizationLogos())
	ensureBucket(ctx, log, storageSvc, "quote-pdfs", cfg.GetMinioBucketQuotePDFs())
	ensureBucket(ctx, log, storageSvc, "quote-attachments", cfg.GetMinioBucketQuoteAttachments())
	log.Info(
		"storage service initialized",
		"leadAttachmentsBucket", cfg.GetMinioBucketLeadServiceAttachments(),
		"catalogAssetsBucket", cfg.GetMinioBucketCatalogAssets(),
		"partnerLogosBucket", cfg.GetMinioBucketPartnerLogos(),
		"organizationLogosBucket", cfg.GetMinioBucketOrganizationLogos(),
		"quotePDFsBucket", cfg.GetMinioBucketQuotePDFs(),
		"quoteAttachmentsBucket", cfg.GetMinioBucketQuoteAttachments(),
	)

	return storageSvc
}

type whatsappagentTranscriberAdapter struct {
	client *transcription.Client
}

func (a whatsappagentTranscriberAdapter) Name() string {
	return a.client.Name()
}

func (a whatsappagentTranscriberAdapter) Transcribe(ctx context.Context, input whatsappagent.AudioTranscriptionInput) (whatsappagent.AudioTranscriptionResult, error) {
	result, err := a.client.Transcribe(ctx, transcription.Input{
		Filename:    input.Filename,
		ContentType: input.ContentType,
		Data:        input.Data,
	})
	if err != nil {
		return whatsappagent.AudioTranscriptionResult{}, err
	}
	return whatsappagent.AudioTranscriptionResult{
		Text:       result.Text,
		Language:   result.Language,
		Confidence: result.Confidence,
	}, nil
}

func initAudioTranscriber(log *logger.Logger) (whatsappagent.AudioTranscriber, func()) {
	noop := func() { /* no model loaded, nothing to close */ }
	client, err := transcription.NewClient()
	if err != nil {
		log.Warn("whisper transcription disabled", "error", err)
		return nil, noop
	}
	log.Info("whisper.cpp transcription initialized", "model", transcription.DefaultModelPath)
	return whatsappagentTranscriberAdapter{client: client}, func() { _ = client.Close() }
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := logger.New(cfg.Env)
	log.Info("starting scheduler", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var pool *pgxpool.Pool
	if err := withRetry(ctx, log, "database connection", 5, 2*time.Second, func() error {
		p, err := db.NewPool(ctx, cfg)
		if err != nil {
			return err
		}
		pool = p
		return nil
	}); err != nil {
		log.Error("failed to connect to database", "error", err)
		panic("failed to connect to database: " + err.Error())
	}
	defer pool.Close()

	eventBus := events.NewInMemoryBus(log)
	orchestratorLockRedis, closeOrchestratorLockRedis := initOrchestratorLockRedis(cfg, log)
	defer closeOrchestratorLockRedis()
	reminderScheduler, closeReminderScheduler := initReminderSchedulerWithCloser(cfg, log)
	defer closeReminderScheduler()

	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic("failed to initialize email sender: " + err.Error())
	}

	notificationModule := notification.New(pool, sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)
	whatsAppClient := whatsapp.NewClient(cfg, log)
	leadReader := leadrepo.New(pool)
	storageSvc := initStorageOrPanic(ctx, cfg, log)
	notificationModule.SetWhatsAppSender(whatsAppClient)
	notificationModule.SetLeadWhatsAppReader(leadReader)
	notificationModule.SetOrganizationMemberReader(leadReader)
	notificationModule.SetNotificationOutbox(outbox.New(pool))
	identityReader := identityrepo.New(pool)
	identitySvc := identityservice.New(
		identityReader,
		leadReader,
		eventBus,
		storageSvc,
		cfg.GetMinioBucketOrganizationLogos(),
		whatsAppClient,
	)
	notificationModule.SetOrganizationSettingsReader(identityReader)
	notificationModule.SetUserTenancyReader(identitySvc)
	notificationModule.SetWorkflowResolver(identitySvc)
	wireSchedulerSMTPEncryptionKey(cfg, log, identitySvc, notificationModule)

	val := validator.New()

	// Worker-side quote generation wiring (no HTTP handlers required).
	catalogModule := catalog.NewModule(pool, storageSvc, cfg.GetMinioBucketCatalogAssets(), val, cfg, log)
	leadsModule, err := leads.NewModule(ctx, pool, eventBus, storageSvc, val, leads.ModuleDeps{
		Config:                cfg,
		Log:                   log,
		OrchestratorLockRedis: orchestratorLockRedis,
	})
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}
	partnersModule := partners.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketPartnerLogos(), val)
	quotesModule := quotes.NewModule(pool, eventBus, val)
	quotesModule.Service().SetLeadTransferCreator(leadsModule.ManagementService())
	quotesModule.Service().SetLeadTransferRepository(leadsModule.Repository())
	tasksModule := tasks.NewModule(pool, val, reminderScheduler, leadsModule.Repository(), log)
	leadsModule.ManagementService().SetAcceptedQuoteUpdater(quotesModule.Service())

	catalogReader := adapters.NewCatalogProductReader(catalogModule.Repository())
	leadsModule.SetCatalogReader(catalogReader)

	quotesDrafter := adapters.NewQuotesDraftWriter(quotesModule.Service())
	leadsModule.SetQuoteDrafter(quotesDrafter)
	leadsModule.SetPricingIntelligenceReader(adapters.NewQuotePricingIntelligenceReader(quotesModule.Repository()))

	leadAssigner := adapters.NewAppointmentsLeadAssigner(leadsModule.ManagementService())
	appointmentsModule := appointments.NewModule(appointments.Dependencies{
		Pool:              pool,
		Validator:         val,
		LeadAssigner:      leadAssigner,
		EmailSender:       sender,
		EventBus:          eventBus,
		ReminderScheduler: reminderScheduler,
		Storage:           storageSvc,
		AttachmentBucket:  cfg.GetMinioBucketLeadServiceAttachments(),
		TimelineRecorder:  leadsModule.Repository(),
	})
	appointmentsModule.SetSSE(leadsModule.SSE())
	appointmentBooker := adapters.NewAppointmentsAdapter(appointmentsModule.Service)
	leadsModule.SetAppointmentBooker(appointmentBooker)
	leadsModule.SetCallLogScheduler(reminderScheduler)
	leadsModule.SetAutomationScheduler(reminderScheduler)

	quoteGenAdapter := adapters.NewQuoteGeneratorAdapter(leadsModule.QuoteGeneratorAgent())
	quotesModule.Service().SetQuotePromptGenerator(quoteGenAdapter)
	partnersModule.Service().SetOfferSummaryGenerator(adapters.NewOfferSummaryGeneratorAdapter(leadsModule.OfferSummaryGenerator()))

	dispatcher, err := scheduler.NewNotificationOutboxDispatcher(cfg, pool, log)
	if err != nil {
		log.Error("failed to initialize outbox dispatcher", "error", err)
		panic("failed to initialize outbox dispatcher: " + err.Error())
	}
	defer func() { _ = dispatcher.Close() }()
	go dispatcher.Run(ctx)

	cleanupInterval := getDurationEnv("AI_QUOTE_JOB_CLEANUP_INTERVAL", time.Hour)
	completedRetention := time.Duration(getPositiveIntEnv("AI_QUOTE_JOB_COMPLETED_RETENTION_DAYS", 14)) * 24 * time.Hour
	failedRetention := time.Duration(getPositiveIntEnv("AI_QUOTE_JOB_FAILED_RETENTION_DAYS", 30)) * 24 * time.Hour
	aiQuoteJobCleanup := scheduler.NewAIQuoteJobCleanup(pool, log, cleanupInterval, completedRetention, failedRetention)
	go aiQuoteJobCleanup.Run(ctx)

	// Periodic catalog gap analyzer ("Librarian"): turns frequent 0-result searches
	// and ad-hoc quote items into draft catalog products for human review.
	gapInterval := getDurationEnv("CATALOG_GAP_ANALYZER_INTERVAL", 6*time.Hour)
	maxDrafts := getPositiveIntEnv("CATALOG_GAP_MAX_DRAFTS_PER_RUN", 10)
	gapAnalyzer := maintenance.NewCatalogGapAnalyzer(leadrepo.New(pool), catalogModule.Repository(), log)
	go runCatalogGapAnalyzerLoop(ctx, pool, gapAnalyzer, gapInterval, maxDrafts, log)

	worker, err := scheduler.NewWorker(cfg, pool, eventBus, log)
	if err != nil {
		log.Error("failed to initialize scheduler worker", "error", err)
		panic("failed to initialize scheduler worker: " + err.Error())
	}
	worker.SetQuoteJobProcessor(quotesModule.Service())
	worker.SetCallLogProcessor(leadsModule)
	worker.SetLeadAutomationProcessor(leadsModule)
	worker.SetOfferSummaryProcessor(partnersModule.Service())
	worker.SetTaskReminderProcessor(tasksModule.Service())
	imapModule := imap.NewModule(pool, val, eventBus, log)
	worker.SetIMAPSyncProcessor(imapModule.Service())
	wireSchedulerIMAPEncryptionKey(cfg, log, imapModule.Service())

	notificationModule.SetQuotePDFStorage(storageSvc, cfg.GetMinioBucketQuotePDFs())
	quoteTermsResolver := adapters.NewQuoteTermsResolverAdapter(identitySvc, identitySvc, leadsModule.Repository())
	quotePDFProcessor := adapters.NewQuoteAcceptanceProcessor(quotesModule.Repository(), identitySvc, nil, storageSvc, cfg, quoteTermsResolver)
	notificationModule.SetQuotePDFGenerator(quotePDFProcessor)
	worker.SetAcceptedQuotePDFProcessor(quotePDFProcessor)
	audioTranscriber, closeTranscriber := initAudioTranscriber(log)
	defer closeTranscriber()
	inboxLeadActions := adapters.NewInboxLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository(), eventBus)
	whatsappagentModule, err := whatsappagent.NewModule(pool, whatsappagent.ModuleConfig{
		MoonshotAPIKey: cfg.MoonshotAPIKey,
		LLMModel:       cfg.ResolveLLMModel(config.LLMModelAgentWhatsAppAgent),
		WebhookSecret:  cfg.GetWhatsAppWebhookSecret(),
	}, whatsappagent.ModuleDependencies{
		WhatsAppClient:               whatsAppClient,
		QuotesReader:                 adapters.NewWhatsAppAgentQuotesAdapter(quotesModule.Service()),
		AppointmentsReader:           adapters.NewWhatsAppAgentAppointmentsAdapter(appointmentsModule.Service),
		LeadSearchReader:             adapters.NewWhatsAppAgentLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository()),
		LeadDetailsReader:            adapters.NewWhatsAppAgentLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository()),
		NavigationLinkReader:         adapters.NewWhatsAppAgentLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository()),
		CatalogSearchReader:          adapters.NewWhatsAppAgentCatalogSearchAdapter(catalogModule.Service(), catalogReader),
		LeadMutationWriter:           adapters.NewWhatsAppAgentLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository()),
		TaskWriter:                   adapters.NewWhatsAppAgentTaskWriterAdapter(tasksModule.Service(), leadsModule.Repository()),
		QuoteWorkflowWriter:          adapters.NewWhatsAppAgentQuoteWorkflowAdapter(quotesModule.Service(), quotePDFProcessor, storageSvc, cfg.GetMinioBucketQuotePDFs()),
		CurrentInboundPhotoAttacher:  adapters.NewWhatsAppAgentCurrentInboundPhotoAdapter(whatsAppClient, storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), inboxLeadActions, whatsappagentdb.New(pool)),
		Storage:                      storageSvc,
		AttachmentBucket:             cfg.GetMinioBucketLeadServiceAttachments(),
		TranscriptionScheduler:       reminderScheduler,
		AudioTranscriber:             audioTranscriber,
		InboxMessageSync:             identitySvc,
		VisitSlotReader:              adapters.NewWhatsAppAgentVisitActionsAdapter(adapters.NewAppointmentSlotAdapter(appointmentsModule.Service), appointmentsModule.Service, leadsModule.Repository()),
		VisitMutationWriter:          adapters.NewWhatsAppAgentVisitActionsAdapter(adapters.NewAppointmentSlotAdapter(appointmentsModule.Service), appointmentsModule.Service, leadsModule.Repository()),
		PartnerPhoneReader:           adapters.NewWhatsAppAgentPartnerAdapter(partnersModule.Service(), appointmentsModule.Service),
		PartnerJobReader:             adapters.NewWhatsAppAgentPartnerAdapter(partnersModule.Service(), appointmentsModule.Service),
		AppointmentVisitReportWriter: adapters.NewWhatsAppAgentPartnerAdapter(partnersModule.Service(), appointmentsModule.Service),
		AppointmentStatusWriter:      adapters.NewWhatsAppAgentPartnerAdapter(partnersModule.Service(), appointmentsModule.Service),
		RedisClient:                  orchestratorLockRedis,
		InboxWriter:                  identitySvc,
		Logger:                       log,
	})
	if err != nil {
		log.Error("failed to initialize whatsappagent module", "error", err)
		panic("failed to initialize whatsappagent module: " + err.Error())
	}
	worker.SetWAAgentVoiceTranscriptionProcessor(whatsappagentModule.Service())

	worker.Run(ctx)
}

func initOrchestratorLockRedis(cfg *config.Config, log *logger.Logger) (*redis.Client, func()) {
	if cfg.GetRedisURL() == "" {
		return nil, noOpRedisCloser
	}

	redisClient, err := rediskit.NewClient(cfg.GetRedisURL(), cfg.GetRedisTLSInsecure())
	if err != nil {
		log.Warn("failed to initialize redis orchestrator lock client", "error", err)
		return nil, noOpRedisCloser
	}

	return redisClient, func() { _ = redisClient.Close() }
}

func initReminderSchedulerWithCloser(cfg *config.Config, log *logger.Logger) (*scheduler.Client, func()) {
	if cfg.GetRedisURL() == "" {
		log.Error("REDIS_URL not configured; async scheduler is required")
		panic("REDIS_URL not configured: async scheduler is required")
	}

	reminderClient, err := scheduler.NewClient(cfg)
	if err != nil {
		log.Error("failed to initialize reminder scheduler client", "error", err)
		panic("failed to initialize reminder scheduler client: " + err.Error())
	}

	return reminderClient, func() {
		_ = reminderClient.Close()
	}
}

type gapOrgSettings struct {
	OrganizationID uuid.UUID
	Threshold      int
	LookbackDays   int
}

func runCatalogGapAnalyzerLoop(ctx context.Context, pool *pgxpool.Pool, analyzer *maintenance.CatalogGapAnalyzer, interval time.Duration, maxDrafts int, log *logger.Logger) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	if maxDrafts <= 0 {
		maxDrafts = 10
	}

	// Run once shortly after startup, then on interval.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Small startup delay to avoid competing with initial DB connection churn.
	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}

	runCatalogGapAnalyzerOnce(ctx, pool, analyzer, maxDrafts, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCatalogGapAnalyzerOnce(ctx, pool, analyzer, maxDrafts, log)
		}
	}
}

func runCatalogGapAnalyzerOnce(ctx context.Context, pool *pgxpool.Pool, analyzer *maintenance.CatalogGapAnalyzer, maxDrafts int, log *logger.Logger) {
	orgs, err := listGapEnabledOrganizations(ctx, pool)
	if err != nil {
		log.Warn("catalog gap: failed to list org settings", "error", err)
		return
	}
	if len(orgs) == 0 {
		return
	}

	for _, o := range orgs {
		if ctx.Err() != nil {
			return
		}
		res, err := analyzer.RunForOrganization(ctx, o.OrganizationID, o.Threshold, o.LookbackDays, maxDrafts)
		if err != nil {
			log.Warn("catalog gap: run failed", "orgId", o.OrganizationID, "error", err)
			continue
		}
		if res.CreatedDrafts > 0 || res.Candidates > 0 {
			log.Info("catalog gap: run completed", "orgId", o.OrganizationID, "candidates", res.Candidates, "createdDrafts", res.CreatedDrafts, "skippedExists", res.SkippedExists)
		}
	}
}

func listGapEnabledOrganizations(ctx context.Context, pool *pgxpool.Pool) ([]gapOrgSettings, error) {
	rows, err := pool.Query(ctx, `
		SELECT organization_id, catalog_gap_threshold, catalog_gap_lookback_days
		FROM RAC_organization_settings
		WHERE catalog_gap_threshold > 0 AND catalog_gap_lookback_days > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]gapOrgSettings, 0)
	for rows.Next() {
		var it gapOrgSettings
		if err := rows.Scan(&it.OrganizationID, &it.Threshold, &it.LookbackDays); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return items, nil
}

func withRetry(ctx context.Context, log *logger.Logger, name string, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		return errors.New(name + ": invalid retry attempts")
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			log.Warn("retryable operation failed", "operation", name, "attempt", attempt, "error", err)
		}

		if attempt < attempts {
			delay := time.Duration(attempt*attempt) * baseDelay
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return errors.New(name + ": " + lastErr.Error())
}

func getPositiveIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func wireSchedulerIMAPEncryptionKey(cfg *config.Config, log *logger.Logger, imapSvc interface{ SetEncryptionKey([]byte) }) {
	keyHex := cfg.GetIMAPEncryptionKey()
	if strings.TrimSpace(keyHex) == "" {
		return
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Error("invalid IMAP_ENCRYPTION_KEY (must be hex-encoded)", "error", err)
		panic("invalid IMAP_ENCRYPTION_KEY: " + err.Error())
	}
	if len(key) != 32 {
		log.Error("IMAP_ENCRYPTION_KEY must be 32 bytes (64 hex chars)", "length", len(key))
		panic("IMAP_ENCRYPTION_KEY must be 32 bytes")
	}
	imapSvc.SetEncryptionKey(key)
	log.Info("scheduler imap encryption key configured")
}

func wireSchedulerSMTPEncryptionKey(cfg *config.Config, log *logger.Logger, identitySvc interface{ SetSMTPEncryptionKey([]byte) }, notificationMod interface{ SetSMTPEncryptionKey([]byte) }) {
	keyHex := cfg.GetSMTPEncryptionKey()
	if strings.TrimSpace(keyHex) == "" {
		return
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Error("invalid SMTP_ENCRYPTION_KEY (must be hex-encoded)", "error", err)
		panic("invalid SMTP_ENCRYPTION_KEY: " + err.Error())
	}
	if len(key) != 32 {
		log.Error("SMTP_ENCRYPTION_KEY must be 32 bytes (64 hex chars)", "length", len(key))
		panic("SMTP_ENCRYPTION_KEY must be 32 bytes")
	}

	identitySvc.SetSMTPEncryptionKey(key)
	notificationMod.SetSMTPEncryptionKey(key)
	log.Info("scheduler smtp encryption key configured")
}
