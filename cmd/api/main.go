package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/appointments"
	"portal_final_backend/internal/auth"
	"portal_final_backend/internal/catalog"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/energylabel"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/exports"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/http/router"
	"portal_final_backend/internal/identity"
	"portal_final_backend/internal/imap"
	"portal_final_backend/internal/leadenrichment"
	"portal_final_backend/internal/leads"
	leadsports "portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/partners"
	"portal_final_backend/internal/pdf"
	"portal_final_backend/internal/quotes"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/search"
	"portal_final_backend/internal/services"
	"portal_final_backend/internal/webhook"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/rediskit"
	"portal_final_backend/platform/validator"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const storageBucketEnsureErrPrefix = "failed to ensure storage bucket exists: "
const storageBucketEnsureErrMsg = "failed to ensure storage bucket exists"

// ensureBucket wraps the retry logic for verifying a MinIO bucket exists.
func ensureBucket(ctx context.Context, log *logger.Logger, storageSvc storage.StorageService, name, bucket string) {
	if err := withRetry(ctx, log, "ensure "+name+" bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, bucket)
	}); err != nil {
		log.Error(storageBucketEnsureErrMsg, "error", err, "bucket", bucket)
		panic(storageBucketEnsureErrPrefix + err.Error())
	}
}

func loadConfigOrPanic() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}
	return cfg
}

func main() {
	cfg := loadConfigOrPanic()
	log := logger.New(cfg.Env)
	log.Info("starting server", "env", cfg.Env, "addr", cfg.HTTPAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runMigrationsOrPanic(ctx, cfg, log)
	pool := initDBPoolOrPanic(ctx, cfg, log)
	defer pool.Close()

	eventBus := events.NewInMemoryBus(log)
	tokenBlocklistRedis, closeTokenBlocklistRedis := initTokenBlocklistRedis(cfg, log)
	defer closeTokenBlocklistRedis()

	reminderScheduler, closeScheduler := initReminderSchedulerWithCloser(cfg, log)
	defer closeScheduler()

	sender := initEmailSenderOrPanic(cfg, log)
	val := validator.New()
	storageSvc := initStorageOrPanic(ctx, cfg, log)
	initGotenbergIfEnabled(cfg, log)

	app := buildHTTPApp(appBuildDeps{
		ctx:                 ctx,
		cfg:                 cfg,
		log:                 log,
		pool:                pool,
		eventBus:            eventBus,
		sender:              sender,
		storageSvc:          storageSvc,
		val:                 val,
		reminderScheduler:   reminderScheduler,
		tokenBlocklistRedis: tokenBlocklistRedis,
	})
	serveUntilShutdown(ctx, cfg, log, eventBus, app)
}

func noOpCloser() {
	// Intentionally empty: used as a safe default closer when no resource was initialized.
}

func runMigrationsOrPanic(ctx context.Context, cfg *config.Config, log *logger.Logger) {
	if err := withRetry(ctx, log, "database migrations", 5, 2*time.Second, func() error {
		return db.RunMigrations(ctx, cfg, "migrations")
	}); err != nil {
		log.Error("failed to run database migrations", "error", err)
		panic("failed to run database migrations: " + err.Error())
	}
	log.Info("database migrations complete")
}

func initDBPoolOrPanic(ctx context.Context, cfg *config.Config, log *logger.Logger) *pgxpool.Pool {
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
	log.Info("database connection established")
	return pool
}

func initTokenBlocklistRedis(cfg *config.Config, log *logger.Logger) (*redis.Client, func()) {
	if cfg.GetRedisURL() == "" {
		return nil, noOpCloser
	}

	redisClient, redisErr := rediskit.NewClient(cfg.GetRedisURL(), cfg.GetRedisTLSInsecure())
	if redisErr != nil {
		log.Warn("failed to initialize redis token blocklist", "error", redisErr)
		return nil, noOpCloser
	}

	httpkit.SetTokenRevocationLookup(func(ctx context.Context, jti string) (bool, error) {
		err := redisClient.Get(ctx, "auth:blocklist:jti:"+jti).Err()
		if err == nil {
			return true, nil
		}
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	})

	return redisClient, func() { _ = redisClient.Close() }
}

func initReminderSchedulerWithCloser(cfg *config.Config, log *logger.Logger) (*scheduler.Client, func()) {
	client, closer := initReminderScheduler(cfg, log)
	if closer == nil {
		return client, noOpCloser
	}
	return client, closer
}

func initEmailSenderOrPanic(cfg *config.Config, log *logger.Logger) email.Sender {
	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic("failed to initialize email sender: " + err.Error())
	}
	return sender
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

type appBuildDeps struct {
	ctx                 context.Context
	cfg                 *config.Config
	log                 *logger.Logger
	pool                *pgxpool.Pool
	eventBus            *events.InMemoryBus
	sender              email.Sender
	storageSvc          storage.StorageService
	val                 *validator.Validator
	reminderScheduler   *scheduler.Client
	tokenBlocklistRedis *redis.Client
}

func buildHTTPApp(deps appBuildDeps) *apphttp.App {
	ctx := deps.ctx
	cfg := deps.cfg
	log := deps.log
	pool := deps.pool
	eventBus := deps.eventBus
	sender := deps.sender
	storageSvc := deps.storageSvc
	val := deps.val
	reminderScheduler := deps.reminderScheduler
	tokenBlocklistRedis := deps.tokenBlocklistRedis

	notificationModule := notification.New(pool, sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)
	notificationModule.SetQuoteAcceptedPDFScheduler(reminderScheduler)
	whatsappClient := whatsapp.NewClient(cfg, log)
	notificationModule.SetWhatsAppSender(whatsappClient)
	notificationModule.SetNotificationOutbox(outbox.New(pool))

	identityModule := identity.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketOrganizationLogos(), val, whatsappClient)
	identityModule.RegisterHandlers(eventBus)
	notificationModule.SetOrganizationSettingsReader(identityModule.Service())
	notificationModule.SetUserTenancyReader(identityModule.Service())
	notificationModule.SetWorkflowResolver(identityModule.Service())

	wireSMTPEncryptionKey(cfg, log, identityModule.Service(), notificationModule)
	imapModule := imap.NewModule(pool, val, eventBus)
	if reminderScheduler != nil {
		imapModule.Service().SetScheduler(reminderScheduler)
		go runIMAPPeriodicSweep(ctx, reminderScheduler, log)
	}

	authModule := auth.NewModule(pool, identityModule.Service(), cfg, eventBus, log, val)
	authModule.Service().SetAccessTokenBlocklistRedis(tokenBlocklistRedis)

	leadsModule, err := leads.NewModule(pool, eventBus, storageSvc, val, cfg, log)
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}
	leadsModule.ManagementService().SetWorkflowOverrideWriter(identityModule.Service())
	leadsModule.ManagementService().SetInAppNotificationService(notificationModule.InAppService())
	leadsModule.SetOrganizationAISettingsReader(func(ctx context.Context, organizationID uuid.UUID) (leadsports.OrganizationAISettings, error) {
		settings, err := identityModule.Service().GetOrganizationSettings(ctx, organizationID)
		if err != nil {
			return leadsports.OrganizationAISettings{}, err
		}
		return leadsports.OrganizationAISettings{
			AIAutoDisqualifyJunk:   settings.AIAutoDisqualifyJunk,
			AIAutoDispatch:         settings.AIAutoDispatch,
			AIAutoEstimate:         settings.AIAutoEstimate,
			CatalogGapThreshold:    settings.CatalogGapThreshold,
			CatalogGapLookbackDays: settings.CatalogGapLookbackDays,
		}, nil
	})
	notificationModule.SetLeadWhatsAppReader(leadsModule.Repository())
	notificationModule.SetOrganizationMemberReader(leadsModule.Repository())
	notificationModule.SetLeadAssigneeReader(adapters.NewLeadAssigneeReader(leadsModule.Repository()))

	notificationModule.SetSSE(leadsModule.SSE())
	leadAssigner := adapters.NewAppointmentsLeadAssigner(leadsModule.ManagementService())
	appointmentsModule := appointments.NewModule(pool, val, leadAssigner, sender, eventBus, reminderScheduler)
	appointmentsModule.SetSSE(leadsModule.SSE())
	appointmentBooker := adapters.NewAppointmentsAdapter(appointmentsModule.Service)
	leadsModule.SetAppointmentBooker(appointmentBooker)
	leadsModule.SetCallLogScheduler(reminderScheduler)

	energyLabelModule := energylabel.NewModule(cfg, log)
	if energyLabelModule.IsEnabled() {
		energyLabelEnricher := adapters.NewEnergyLabelAdapter(energyLabelModule.Service())
		leadsModule.SetEnergyLabelEnricher(energyLabelEnricher)
	}

	leadEnrichmentModule := leadenrichment.NewModule(log)
	leadsModule.SetLeadEnricher(adapters.NewLeadEnrichmentAdapter(leadEnrichmentModule.Service()))

	mapsModule := maps.NewModule(log)
	servicesModule := services.NewModule(pool, val, log)
	servicesModule.RegisterHandlers(eventBus)
	catalogModule := catalog.NewModule(pool, storageSvc, cfg.GetMinioBucketCatalogAssets(), val, cfg, log)
	catalogModule.RegisterHandlers(eventBus)
	partnersModule := partners.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketPartnerLogos(), val)
	quotesModule := quotes.NewModule(pool, eventBus, val)
	searchModule := search.NewModule(pool, val)
	quotesModule.SetGenerateQuoteJobQueue(reminderScheduler)
	wireMoneybirdConfig(cfg, log, quotesModule.Service())

	leadsModule.SetPublicViewers(
		adapters.NewQuotePublicAdapter(quotesModule.Service()),
		adapters.NewAppointmentPublicAdapter(appointmentsModule.Service),
		adapters.NewAppointmentSlotAdapter(appointmentsModule.Service),
	)
	leadsModule.SetPublicOrgViewer(adapters.NewOrganizationPublicAdapter(identityModule.Service()))
	leadsModule.SetPartnerOfferCreator(adapters.NewPartnerOfferAdapter(partnersModule.Service()))
	partnersModule.Service().SetOfferSummaryGenerator(adapters.NewOfferSummaryGeneratorAdapter(leadsModule.OfferSummaryGenerator()))

	quotesModule.SetSSE(leadsModule.SSE())
	quotesModule.SetStorageForPDF(storageSvc, cfg.GetMinioBucketQuotePDFs())
	quotesModule.SetAttachmentBucket(cfg.GetMinioBucketQuoteAttachments())
	quotesModule.SetCatalogBucket(cfg.GetMinioBucketCatalogAssets())
	quotesModule.Service().SetTimelineWriter(adapters.NewQuotesTimelineWriter(leadsModule.Repository()))

	quotesContacts := adapters.NewQuotesContactReader(leadsModule.Repository(), identityModule.Service(), authModule.Repository())
	quotesModule.Service().SetQuoteContactReader(quotesContacts)
	quotesModule.SetLogoPresigner(adapters.NewQuotesLogoPresigner(storageSvc, cfg.GetMinioBucketOrganizationLogos()))

	quoteTermsResolver := adapters.NewQuoteTermsResolverAdapter(identityModule.Service(), identityModule.Service(), leadsModule.Repository())
	quotesModule.Service().SetQuoteTermsResolver(quoteTermsResolver)
	quotePDFProcessor := adapters.NewQuoteAcceptanceProcessor(quotesModule.Repository(), identityModule.Service(), quotesContacts, storageSvc, cfg, quoteTermsResolver)
	quotesModule.SetPDFGenerator(quotePDFProcessor)
	notificationModule.SetQuotePDFGenerator(quotePDFProcessor)

	notificationModule.SetQuoteActivityWriter(adapters.NewQuoteActivityWriter(quotesModule.Repository()))
	notificationModule.SetOfferTimelineWriter(adapters.NewPartnerOffersTimelineWriter(leadsModule.Repository()))
	notificationModule.SetLeadTimelineWriter(adapters.NewLeadTimelineWriter(leadsModule.Repository()))
	leadsModule.SetCatalogReader(adapters.NewCatalogProductReader(catalogModule.Repository()))
	leadsModule.SetQuoteDrafter(adapters.NewQuotesDraftWriter(quotesModule.Service()))
	quotesModule.Service().SetQuotePromptGenerator(adapters.NewQuoteGeneratorAdapter(leadsModule))

	webhookModule := webhook.NewModule(pool, leadsModule.ManagementService(), storageSvc, cfg.GetMinioBucketLeadServiceAttachments(), eventBus, val, log)
	exportsModule := exports.NewModule(pool, val)
	wireExportsEncryptionKey(cfg, log, exportsModule)

	wireIMAPEncryptionKey(cfg, log, imapModule.Service())
	wireSMTPEncryptionKeyForIMAP(cfg, log, imapModule.Service())

	return &apphttp.App{
		Config:   cfg,
		Logger:   log,
		Health:   db.NewPoolAdapter(pool),
		EventBus: eventBus,
		Modules: []apphttp.Module{
			notificationModule,
			authModule,
			identityModule,
			imapModule,
			leadsModule,
			mapsModule,
			servicesModule,
			catalogModule,
			appointmentsModule,
			partnersModule,
			quotesModule,
			searchModule,
			webhookModule,
			exportsModule,
		},
	}
}

func serveUntilShutdown(ctx context.Context, cfg *config.Config, log *logger.Logger, eventBus *events.InMemoryBus, app *apphttp.App) {
	engine := router.New(app)
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: engine}

	srvErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.HTTPAddr)
		srvErr <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received, gracefully shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("http server shutdown failed", "error", err)
		}
		if err := eventBus.Shutdown(shutdownCtx); err != nil {
			log.Error("event bus shutdown timed out", "error", err)
		}
	case err := <-srvErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			panic("server error: " + err.Error())
		}
	}
}

// wireSMTPEncryptionKey parses and injects the SMTP encryption key into identity and notification modules.
func wireSMTPEncryptionKey(cfg *config.Config, log *logger.Logger, identitySvc interface{ SetSMTPEncryptionKey([]byte) }, notificationMod interface{ SetSMTPEncryptionKey([]byte) }) {
	smtpKeyHex := cfg.GetSMTPEncryptionKey()
	if smtpKeyHex == "" {
		return
	}

	smtpKey, err := hex.DecodeString(smtpKeyHex)
	if err != nil {
		log.Error("invalid SMTP_ENCRYPTION_KEY (must be hex-encoded)", "error", err)
		panic("invalid SMTP_ENCRYPTION_KEY: " + err.Error())
	}
	if len(smtpKey) != 32 {
		log.Error("SMTP_ENCRYPTION_KEY must be 32 bytes (64 hex chars)", "length", len(smtpKey))
		panic("SMTP_ENCRYPTION_KEY must be 32 bytes")
	}

	identitySvc.SetSMTPEncryptionKey(smtpKey)
	notificationMod.SetSMTPEncryptionKey(smtpKey)
	log.Info("smtp encryption key configured")
}

func wireMoneybirdConfig(cfg *config.Config, log *logger.Logger, quotesSvc interface {
	SetMoneybirdConfig(string, string, string, string)
	SetMoneybirdEncryptionKey([]byte)
}) {
	clientID := cfg.GetMoneybirdClientID()
	clientSecret := cfg.GetMoneybirdClientSecret()
	redirectURI := cfg.GetMoneybirdRedirectURI()
	frontendURL := cfg.GetMoneybirdFrontendURL()
	encryptionKeyHex := cfg.GetMoneybirdEncryptionKey()

	if clientID == "" && clientSecret == "" && redirectURI == "" && encryptionKeyHex == "" {
		return
	}

	if clientID == "" || clientSecret == "" || redirectURI == "" || encryptionKeyHex == "" {
		log.Warn("moneybird config is partially configured; oauth flow will be disabled")
		return
	}

	quotesSvc.SetMoneybirdConfig(clientID, clientSecret, redirectURI, frontendURL)

	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		log.Error("invalid MONEYBIRD_ENCRYPTION_KEY (must be hex-encoded)", "error", err)
		panic("invalid MONEYBIRD_ENCRYPTION_KEY: " + err.Error())
	}
	if len(encryptionKey) != 32 {
		log.Error("MONEYBIRD_ENCRYPTION_KEY must be 32 bytes (64 hex chars)", "length", len(encryptionKey))
		panic("MONEYBIRD_ENCRYPTION_KEY must be 32 bytes")
	}

	quotesSvc.SetMoneybirdEncryptionKey(encryptionKey)
	log.Info("moneybird oauth configuration enabled")
}

func wireExportsEncryptionKey(cfg *config.Config, log *logger.Logger, exportsMod interface{ SetEncryptionKey([]byte) }) {
	keyHex := cfg.GetExportsEncryptionKey()
	if keyHex == "" {
		return
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Error("invalid EXPORTS_ENCRYPTION_KEY (must be hex-encoded)", "error", err)
		panic("invalid EXPORTS_ENCRYPTION_KEY: " + err.Error())
	}
	if len(key) != 32 {
		log.Error("EXPORTS_ENCRYPTION_KEY must be 32 bytes (64 hex chars)", "length", len(key))
		panic("EXPORTS_ENCRYPTION_KEY must be 32 bytes")
	}

	exportsMod.SetEncryptionKey(key)
	log.Info("exports encryption key configured")
}

func wireIMAPEncryptionKey(cfg *config.Config, log *logger.Logger, imapSvc interface{ SetEncryptionKey([]byte) }) {
	keyHex := cfg.GetIMAPEncryptionKey()
	if keyHex == "" {
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
	log.Info("imap encryption key configured")
}

func wireSMTPEncryptionKeyForIMAP(cfg *config.Config, log *logger.Logger, imapSvc interface{ SetSMTPEncryptionKey([]byte) }) {
	smtpKeyHex := cfg.GetSMTPEncryptionKey()
	if smtpKeyHex == "" {
		return
	}
	smtpKey, err := hex.DecodeString(smtpKeyHex)
	if err != nil {
		log.Error("invalid SMTP_ENCRYPTION_KEY for imap (must be hex-encoded)", "error", err)
		panic("invalid SMTP_ENCRYPTION_KEY for imap: " + err.Error())
	}
	if len(smtpKey) != 32 {
		log.Error("SMTP_ENCRYPTION_KEY for imap must be 32 bytes (64 hex chars)", "length", len(smtpKey))
		panic("SMTP_ENCRYPTION_KEY for imap must be 32 bytes")
	}
	imapSvc.SetSMTPEncryptionKey(smtpKey)
	log.Info("imap smtp encryption key configured")
}

func initReminderScheduler(cfg config.SchedulerConfig, log *logger.Logger) (*scheduler.Client, func()) {
	if cfg.GetRedisURL() == "" {
		log.Warn("REDIS_URL not configured; appointment reminders and async quote generation are disabled")
		return nil, nil
	}

	reminderClient, err := scheduler.NewClient(cfg)
	if err != nil {
		log.Error("failed to initialize reminder scheduler client", "error", err)
		return nil, nil
	}

	return reminderClient, func() {
		_ = reminderClient.Close()
	}
}

func runIMAPPeriodicSweep(ctx context.Context, schedulerClient *scheduler.Client, log *logger.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := schedulerClient.EnqueueIMAPSyncSweep(ctx); err != nil {
				log.Warn("failed to enqueue periodic imap sync sweep", "error", err)
			}
		}
	}
}

func withRetry(ctx context.Context, log *logger.Logger, name string, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		return fmt.Errorf("%s: invalid retry attempts", name)
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
