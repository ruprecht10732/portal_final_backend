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
	leadagent "portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/maintenance"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/partners"
	partnersrepo "portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/pdf"
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

func initGotenbergIfEnabled(cfg *config.Config, log *logger.Logger) {
	if !cfg.IsGotenbergEnabled() {
		return
	}

	pdf.Init(cfg.GetGotenbergURL(), cfg.GetGotenbergUsername(), cfg.GetGotenbergPassword())
	log.Info("gotenberg PDF generator initialized", "url", cfg.GetGotenbergURL())
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
	initGotenbergIfEnabled(cfg, log)

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
	leadsModule.GetSubsidyAnalyzerService().SetSchedulerClient(*reminderScheduler)
	leadsModule.GetSubsidyAnalyzerService().SetQuoteRepo(*quotesModule.Repository())
	quotesModule.SetSubsidyAnalyzerService(leadsModule.GetSubsidyAnalyzerService())
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

	// Morning daily digest: sends a summary email to admin users each morning.
	digestHour := getPositiveIntEnv("DAILY_DIGEST_HOUR", 7)
	staleDetector := maintenance.NewStaleLeadDetector(pool, log)
	digestService := maintenance.NewDailyDigestService(pool, staleDetector, log)
	go runDailyDigestLoop(ctx, pool, digestService, sender, digestHour, cfg, log)

	// Stale lead in-app notification sweep: enqueues per-lead notifications for
	// all organisations so agents are nudged about leads that have gone quiet.
	staleNotifier := maintenance.NewStaleLeadNotifier(pool, notificationModule.InAppService(), log)
	staleLeadSweepInterval := getDurationEnv("STALE_LEAD_SWEEP_INTERVAL", 4*time.Hour)

	worker, err := scheduler.NewWorker(cfg, pool, eventBus, log)
	if err != nil {
		log.Error("failed to initialize scheduler worker", "error", err)
		panic("failed to initialize scheduler worker: " + err.Error())
	}
	worker.SetQuoteJobProcessor(quotesModule.Service())
	worker.SetCallLogProcessor(leadsModule)
	worker.SetLeadAutomationProcessor(leadsModule)
	worker.SetSubsidyAnalyzerProcessor(leadsModule.GetSubsidyAnalyzerService())
	worker.SetStaleLeadNotifyProcessor(staleNotifier)
	worker.SetStaleLeadReEngageProcessor(leadsModule.StaleLeadReEngagement())
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

	offerPDFProcessor := adapters.NewPartnerOfferPDFProcessor(partnersrepo.New(pool), identitySvc, storageSvc, cfg, sender)
	worker.SetOfferPDFProcessor(offerPDFProcessor)
	audioTranscriber, closeTranscriber := initAudioTranscriber(log)
	defer closeTranscriber()
	inboxLeadActions := adapters.NewInboxLeadActionsAdapter(leadsModule.ManagementService(), leadsModule.Repository(), eventBus)
	whatsappagentModule, err := whatsappagent.NewModule(pool, whatsappagent.ModuleConfig{
		ModelConfig:   leadagent.NewProviderModelConfig(cfg.ResolveProviderConfig(cfg.LLMProvider), true, cfg.ResolveLLMModel(config.LLMModelAgentWhatsAppAgent)),
		WebhookSecret: cfg.GetWhatsAppWebhookSecret(),
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
		TaskReader:                   adapters.NewWhatsAppAgentTaskReaderAdapter(tasksModule.Service()),
		EnergyLabelReader:            adapters.NewWhatsAppAgentEnergyLabelAdapter(nil),
		ISDECalculator:               adapters.NewWhatsAppAgentISDEAdapter(nil),
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

	go runStaleLeadSweepLoop(ctx, pool, staleDetector, reminderScheduler, reminderScheduler, staleLeadSweepInterval, log)

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

type digestOrg struct {
	OrganizationID uuid.UUID
	Name           string
	DigestEnabled  bool
}

type digestAdminUser struct {
	Email string
}

func runDailyDigestLoop(
	ctx context.Context,
	pool *pgxpool.Pool,
	digestService *maintenance.DailyDigestService,
	sender email.Sender,
	targetHour int,
	cfg *config.Config,
	log *logger.Logger,
) {
	loc, _ := time.LoadLocation("Europe/Amsterdam")

	// Check every 15 minutes if it's time to send.
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	var lastSentDate string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().In(loc)
			todayStr := now.Format("2006-01-02")
			if now.Hour() == targetHour && lastSentDate != todayStr {
				lastSentDate = todayStr
				runDailyDigestOnce(ctx, pool, digestService, sender, cfg, log)
			}
		}
	}
}

func runDailyDigestOnce(
	ctx context.Context,
	pool *pgxpool.Pool,
	digestService *maintenance.DailyDigestService,
	sender email.Sender,
	cfg *config.Config,
	log *logger.Logger,
) {
	orgs, err := listDigestEnabledOrganizations(ctx, pool)
	if err != nil {
		log.Warn("daily digest: failed to list organizations", "error", err)
		return
	}

	dashboardURL := cfg.GetAppBaseURL() + "/dashboard"

	for _, org := range orgs {
		if ctx.Err() != nil {
			return
		}
		sendDigestForOrg(ctx, pool, digestService, sender, org, dashboardURL, log)
	}
}

// runStaleLeadSweepLoop periodically detects stale lead services across all
// organisations and enqueues a per-service notification task. Tasks are
// deduplicated by asynq (unique TTL = 24 h) so duplicate runs are safe.
// It also enqueues AI re-engagement suggestion tasks.
func runStaleLeadSweepLoop(
	ctx context.Context,
	pool *pgxpool.Pool,
	detector *maintenance.StaleLeadDetector,
	notifySched scheduler.StaleLeadNotifyScheduler,
	reEngageSched scheduler.StaleLeadReEngageScheduler,
	interval time.Duration,
	log *logger.Logger,
) {
	if interval <= 0 {
		interval = 4 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Small startup delay so the worker is fully up before the first sweep.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	runStaleLeadSweepOnce(ctx, pool, detector, notifySched, reEngageSched, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runStaleLeadSweepOnce(ctx, pool, detector, notifySched, reEngageSched, log)
		}
	}
}

func runStaleLeadSweepOnce(
	ctx context.Context,
	pool *pgxpool.Pool,
	detector *maintenance.StaleLeadDetector,
	notifySched scheduler.StaleLeadNotifyScheduler,
	reEngageSched scheduler.StaleLeadReEngageScheduler,
	log *logger.Logger,
) {
	orgs, err := listAllOrganizations(ctx, pool)
	if err != nil {
		log.Warn("stale lead sweep: failed to list organizations", "error", err)
		return
	}
	if len(orgs) == 0 {
		return
	}

	enqueued := 0
	reEngaged := 0
	for _, orgID := range orgs {
		if ctx.Err() != nil {
			return
		}
		items, err := detector.ListStaleLeadServices(ctx, orgID, 100)
		if err != nil {
			log.Warn("stale lead sweep: detector failed", "orgId", orgID, "error", err)
			continue
		}
		orgReEngaged := 0
		for _, item := range items {
			payload := scheduler.StaleLeadNotifyPayload{
				OrganizationID:    item.OrganizationID.String(),
				LeadID:            item.LeadID.String(),
				LeadServiceID:     item.ServiceID.String(),
				StaleReason:       string(item.StaleReason),
				ConsumerFirstName: item.ConsumerFirstName,
				ConsumerLastName:  item.ConsumerLastName,
				ServiceType:       item.ServiceType,
				PipelineStage:     item.PipelineStage,
			}
			if err := notifySched.EnqueueStaleLeadNotify(ctx, payload); err != nil {
				log.Warn("stale lead sweep: enqueue failed", "serviceId", item.ServiceID, "error", err)
				continue
			}
			enqueued++

			// Also enqueue an AI re-engagement suggestion task (capped at 20 per org).
			if reEngageSched != nil && orgReEngaged < 20 {
				rePayload := scheduler.StaleLeadReEngagePayload{
					OrganizationID: item.OrganizationID.String(),
					LeadID:         item.LeadID.String(),
					LeadServiceID:  item.ServiceID.String(),
					StaleReason:    string(item.StaleReason),
				}
				if err := reEngageSched.EnqueueStaleLeadReEngage(ctx, rePayload); err != nil {
					log.Warn("stale lead sweep: re-engage enqueue failed", "serviceId", item.ServiceID, "error", err)
				} else {
					orgReEngaged++
					reEngaged++
				}
			}
		}
	}
	if enqueued > 0 {
		log.Info("stale lead sweep: completed", "enqueued", enqueued, "reEngaged", reEngaged)
	}
}

func listAllOrganizations(ctx context.Context, pool *pgxpool.Pool) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `SELECT id FROM RAC_organizations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func sendDigestForOrg(
	ctx context.Context,
	pool *pgxpool.Pool,
	digestService *maintenance.DailyDigestService,
	sender email.Sender,
	org digestOrg,
	dashboardURL string,
	log *logger.Logger,
) {
	if !org.DigestEnabled {
		return
	}

	digest, err := digestService.GenerateDigest(ctx, org.OrganizationID, org.Name, dashboardURL)
	if err != nil {
		log.Warn("daily digest: generation failed", "orgId", org.OrganizationID, "error", err)
		return
	}

	admins, err := listOrganizationAdmins(ctx, pool, org.OrganizationID)
	if err != nil {
		log.Warn("daily digest: failed to list admins", "orgId", org.OrganizationID, "error", err)
		return
	}

	staleLeadInputs := make([]email.DailyDigestStaleLeadInput, len(digest.StaleLeads))
	for i, sl := range digest.StaleLeads {
		staleLeadInputs[i] = email.DailyDigestStaleLeadInput{
			ConsumerFirstName: sl.ConsumerFirstName,
			ConsumerLastName:  sl.ConsumerLastName,
			ServiceType:       sl.ServiceType,
			PipelineStage:     sl.PipelineStage,
			StaleReason:       string(sl.StaleReason),
		}
	}

	input := email.DailyDigestInput{
		OrganizationName:           digest.OrganizationName,
		Date:                       digest.Date,
		GatekeeperRuns:             digest.AIActivity.GatekeeperRuns,
		EstimatorRuns:              digest.AIActivity.EstimatorRuns,
		DispatcherRuns:             digest.AIActivity.DispatcherRuns,
		QuotesGenerated:            digest.AIActivity.QuotesGenerated,
		PhotosAnalyzed:             digest.AIActivity.PhotosAnalyzed,
		OffersProcessed:            digest.AIActivity.OffersProcessed,
		StaleLeads:                 staleLeadInputs,
		PipelineTriage:             digest.PipelineSnapshot.Triage,
		PipelineNurturing:          digest.PipelineSnapshot.Nurturing,
		PipelineEstimation:         digest.PipelineSnapshot.Estimation,
		PipelineProposal:           digest.PipelineSnapshot.Proposal,
		PipelineFulfillment:        digest.PipelineSnapshot.Fulfillment,
		PipelineManualIntervention: digest.PipelineSnapshot.ManualIntervention,
		DashboardURL:               dashboardURL,
	}

	sentCount := 0
	for _, admin := range admins {
		if err := sender.SendDailyDigestEmail(ctx, admin.Email, input); err != nil {
			log.Warn("daily digest: send failed", "orgId", org.OrganizationID, "email", admin.Email, "error", err)
			continue
		}
		sentCount++
	}

	if sentCount > 0 {
		log.Info("daily digest: sent", "orgId", org.OrganizationID, "recipients", sentCount, "staleLeads", digest.StaleLeadCount)
	}
}

func listDigestEnabledOrganizations(ctx context.Context, pool *pgxpool.Pool) ([]digestOrg, error) {
	rows, err := pool.Query(ctx, `
		SELECT o.id, o.name, COALESCE(s.daily_digest_enabled, true)
		FROM RAC_organizations o
		LEFT JOIN RAC_organization_settings s ON s.organization_id = o.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []digestOrg
	for rows.Next() {
		var it digestOrg
		if err := rows.Scan(&it.OrganizationID, &it.Name, &it.DigestEnabled); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func listOrganizationAdmins(ctx context.Context, pool *pgxpool.Pool, organizationID uuid.UUID) ([]digestAdminUser, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.email
		FROM RAC_users u
		JOIN RAC_organization_members m ON m.user_id = u.id
		WHERE m.organization_id = $1
			AND m.role = 'admin'
			AND u.email IS NOT NULL
			AND u.email <> ''
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []digestAdminUser
	for rows.Next() {
		var it digestAdminUser
		if err := rows.Scan(&it.Email); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
