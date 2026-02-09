package main

import (
	"context"
	"errors"
	"fmt"
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
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/http/router"
	"portal_final_backend/internal/identity"
	"portal_final_backend/internal/leadenrichment"
	"portal_final_backend/internal/leads"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/partners"
	"portal_final_backend/internal/pdf"
	"portal_final_backend/internal/quotes"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/services"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// Initialize structured logger
	log := logger.New(cfg.Env)
	log.Info("starting server", "env", cfg.Env, "addr", cfg.HTTPAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ========================================================================
	// Infrastructure Layer
	// ========================================================================

	if err := withRetry(ctx, log, "database migrations", 5, 2*time.Second, func() error {
		return db.RunMigrations(ctx, cfg, "migrations")
	}); err != nil {
		log.Error("failed to run database migrations", "error", err)
		panic("failed to run database migrations: " + err.Error())
	}
	log.Info("database migrations complete")

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
	log.Info("database connection established")

	// Event bus for decoupled communication between modules
	eventBus := events.NewInMemoryBus(log)

	reminderScheduler, closeScheduler := initReminderScheduler(cfg, log)
	if closeScheduler != nil {
		defer closeScheduler()
	}

	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic("failed to initialize email sender: " + err.Error())
	}

	// Shared validator instance for dependency injection
	val := validator.New()

	// Storage service for file uploads (MinIO)
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

	// Gotenberg PDF generator
	if cfg.IsGotenbergEnabled() {
		pdf.Init(cfg.GetGotenbergURL(), cfg.GetGotenbergUsername(), cfg.GetGotenbergPassword())
		log.Info("gotenberg PDF generator initialized", "url", cfg.GetGotenbergURL())
	}

	// ========================================================================
	// Domain Modules (Composition Root)
	// ========================================================================

	// Notification module subscribes to domain events (not HTTP-facing)
	notificationModule := notification.New(sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)
	whatsappClient := whatsapp.NewClient(cfg, log)
	notificationModule.SetWhatsAppSender(whatsappClient)

	// Initialize domain modules
	identityModule := identity.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketOrganizationLogos(), val)
	authModule := auth.NewModule(pool, identityModule.Service(), cfg, eventBus, log, val)
	leadsModule, err := leads.NewModule(pool, eventBus, storageSvc, val, cfg, log)
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}

	// Share SSE service with notification module so quote events reach agents
	notificationModule.SetSSE(leadsModule.SSE())
	leadAssigner := adapters.NewAppointmentsLeadAssigner(leadsModule.ManagementService())
	appointmentsModule := appointments.NewModule(pool, val, leadAssigner, sender, eventBus, reminderScheduler)
	appointmentsModule.SetSSE(leadsModule.SSE())

	// Set appointment booker on leads module (breaks circular dependency)
	appointmentBooker := adapters.NewAppointmentsAdapter(appointmentsModule.Service)
	leadsModule.SetAppointmentBooker(appointmentBooker)

	// Energy label module for lead enrichment
	energyLabelModule := energylabel.NewModule(cfg, log)
	if energyLabelModule.IsEnabled() {
		energyLabelEnricher := adapters.NewEnergyLabelAdapter(energyLabelModule.Service())
		leadsModule.SetEnergyLabelEnricher(energyLabelEnricher)
	}

	// Lead enrichment module for PDOK/CBS signals
	leadEnrichmentModule := leadenrichment.NewModule(log)
	leadEnricher := adapters.NewLeadEnrichmentAdapter(leadEnrichmentModule.Service())
	leadsModule.SetLeadEnricher(leadEnricher)

	mapsModule := maps.NewModule(log)
	servicesModule := services.NewModule(pool, val, log)
	servicesModule.RegisterHandlers(eventBus)
	catalogModule := catalog.NewModule(pool, storageSvc, cfg.GetMinioBucketCatalogAssets(), val, cfg, log)
	partnersModule := partners.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketPartnerLogos(), val)
	quotesModule := quotes.NewModule(pool, eventBus, val)

	// Wire public viewers for lead portal (quotes + appointments)
	quotePublicViewer := adapters.NewQuotePublicAdapter(quotesModule.Service())
	appointmentPublicViewer := adapters.NewAppointmentPublicAdapter(appointmentsModule.Service)
	appointmentSlotViewer := adapters.NewAppointmentSlotAdapter(appointmentsModule.Service)
	leadsModule.SetPublicViewers(quotePublicViewer, appointmentPublicViewer, appointmentSlotViewer)

	orgPublicViewer := adapters.NewOrganizationPublicAdapter(identityModule.Service())
	leadsModule.SetPublicOrgViewer(orgPublicViewer)

	offerAdapter := adapters.NewPartnerOfferAdapter(partnersModule.Service())
	leadsModule.SetPartnerOfferCreator(offerAdapter)

	offerSummaryAdapter := adapters.NewOfferSummaryGeneratorAdapter(leadsModule.OfferSummaryGenerator())
	partnersModule.Service().SetOfferSummaryGenerator(offerSummaryAdapter)

	// Share SSE service with quotes module so public viewers get real-time updates
	quotesModule.SetSSE(leadsModule.SSE())

	// Inject storage for PDF download endpoints
	quotesModule.SetStorageForPDF(storageSvc, cfg.GetMinioBucketQuotePDFs())

	// Inject bucket for manual quote attachment uploads
	quotesModule.SetAttachmentBucket(cfg.GetMinioBucketQuoteAttachments())

	// Inject catalog bucket for attachment preview (catalog-sourced docs)
	quotesModule.SetCatalogBucket(cfg.GetMinioBucketCatalogAssets())

	// Wire timeline integration: quotes → leads timeline
	quotesTimeline := adapters.NewQuotesTimelineWriter(leadsModule.Repository())
	quotesModule.Service().SetTimelineWriter(quotesTimeline)

	// Wire contact reader: quotes → leads + identity + auth (for email enrichment)
	quotesContacts := adapters.NewQuotesContactReader(leadsModule.Repository(), identityModule.Service(), authModule.Repository())
	quotesModule.Service().SetQuoteContactReader(quotesContacts)

	// Wire org settings reader: quotes → identity settings (for quote defaults)
	orgSettingsAdapter := adapters.NewOrgSettingsAdapter(identityModule.Service())
	quotesModule.Service().SetOrgSettingsReader(orgSettingsAdapter)

	// Wire quote acceptance processor: PDF generation + upload + emails
	quotePDFProcessor := adapters.NewQuoteAcceptanceProcessor(quotesModule.Repository(), identityModule.Service(), quotesContacts, storageSvc, cfg, identityModule.Service())
	notificationModule.SetQuoteAcceptanceProcessor(quotePDFProcessor)
	quotesModule.SetPDFGenerator(quotePDFProcessor)

	// Wire quote activity writer so notification handlers persist activity history
	quoteActivityWriter := adapters.NewQuoteActivityWriter(quotesModule.Repository())
	notificationModule.SetQuoteActivityWriter(quoteActivityWriter)

	// Wire partner-offer timeline writer so offer events create lead timeline entries
	offerTimelineWriter := adapters.NewPartnerOffersTimelineWriter(leadsModule.Repository())
	notificationModule.SetOfferTimelineWriter(offerTimelineWriter)

	// Wire lead timeline writer for generic lead events (e.g., WhatsApp sent)
	leadTimelineWriter := adapters.NewLeadTimelineWriter(leadsModule.Repository())
	notificationModule.SetLeadTimelineWriter(leadTimelineWriter)

	// Anti-Corruption Layer: Create adapter for cross-domain communication
	// This ensures leads module only depends on its own AgentProvider interface
	_ = adapters.NewAuthAgentProvider(authModule.Service())

	// Wire catalog reader: leads → catalog (for hydrating product search results)
	catalogReader := adapters.NewCatalogProductReader(catalogModule.Repository())
	leadsModule.SetCatalogReader(catalogReader)

	// Wire quote drafter: leads → quotes (for AI-drafted quotes)
	quotesDrafter := adapters.NewQuotesDraftWriter(quotesModule.Service())
	leadsModule.SetQuoteDrafter(quotesDrafter)

	// Wire prompt-based quote generator: quotes → leads (for /quotes/generate endpoint)
	quoteGenAdapter := adapters.NewQuoteGeneratorAdapter(leadsModule)
	quotesModule.Service().SetQuotePromptGenerator(quoteGenAdapter)

	// ========================================================================
	// HTTP Layer
	// ========================================================================

	app := &apphttp.App{
		Config:   cfg,
		Logger:   log,
		Health:   db.NewPoolAdapter(pool),
		EventBus: eventBus,
		Modules: []apphttp.Module{
			authModule,
			identityModule,
			leadsModule,
			mapsModule,
			servicesModule,
			catalogModule,
			appointmentsModule,
			partnersModule,
			quotesModule,
		},
	}

	engine := router.New(app)

	srvErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.HTTPAddr)
		srvErr <- engine.Run(cfg.HTTPAddr)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received, gracefully shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = shutdownCtx
	case err := <-srvErr:
		if err != nil {
			log.Error("server error", "error", err)
			panic("server error: " + err.Error())
		}
	}
}

func initReminderScheduler(cfg config.SchedulerConfig, log *logger.Logger) (scheduler.ReminderScheduler, func()) {
	if cfg.GetRedisURL() == "" {
		log.Warn("REDIS_URL not configured; appointment reminders disabled")
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
