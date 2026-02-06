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
	"portal_final_backend/internal/services"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

const storageBucketEnsureErrPrefix = "failed to ensure storage bucket exists: "
const storageBucketEnsureErrMsg = "failed to ensure storage bucket exists"

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

	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic("failed to initialize email sender: " + err.Error())
	}

	// Event bus for decoupled communication between modules
	eventBus := events.NewInMemoryBus(log)

	// Shared validator instance for dependency injection
	val := validator.New()

	// Storage service for file uploads (MinIO)
	storageSvc, err := storage.NewMinIOService(cfg)
	if err != nil {
		log.Error("failed to initialize storage service", "error", err)
		panic("failed to initialize storage service: " + err.Error())
	}
	// Ensure the lead-service-attachments bucket exists
	if err := withRetry(ctx, log, "ensure lead-service-attachments bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, cfg.GetMinioBucketLeadServiceAttachments())
	}); err != nil {
		log.Error(storageBucketEnsureErrMsg, "error", err, "bucket", cfg.GetMinioBucketLeadServiceAttachments())
		panic(storageBucketEnsureErrPrefix + err.Error())
	}
	// Ensure the catalog assets bucket exists
	if err := withRetry(ctx, log, "ensure catalog assets bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, cfg.GetMinioBucketCatalogAssets())
	}); err != nil {
		log.Error(storageBucketEnsureErrMsg, "error", err, "bucket", cfg.GetMinioBucketCatalogAssets())
		panic(storageBucketEnsureErrPrefix + err.Error())
	}
	// Ensure the partner logos bucket exists
	if err := withRetry(ctx, log, "ensure partner logos bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, cfg.GetMinioBucketPartnerLogos())
	}); err != nil {
		log.Error(storageBucketEnsureErrMsg, "error", err, "bucket", cfg.GetMinioBucketPartnerLogos())
		panic(storageBucketEnsureErrPrefix + err.Error())
	}
	// Ensure the organization logos bucket exists
	if err := withRetry(ctx, log, "ensure organization logos bucket", 5, 2*time.Second, func() error {
		return storageSvc.EnsureBucketExists(ctx, cfg.GetMinioBucketOrganizationLogos())
	}); err != nil {
		log.Error(storageBucketEnsureErrMsg, "error", err, "bucket", cfg.GetMinioBucketOrganizationLogos())
		panic(storageBucketEnsureErrPrefix + err.Error())
	}
	log.Info(
		"storage service initialized",
		"leadAttachmentsBucket", cfg.GetMinioBucketLeadServiceAttachments(),
		"catalogAssetsBucket", cfg.GetMinioBucketCatalogAssets(),
		"partnerLogosBucket", cfg.GetMinioBucketPartnerLogos(),
		"organizationLogosBucket", cfg.GetMinioBucketOrganizationLogos(),
	)

	// ========================================================================
	// Domain Modules (Composition Root)
	// ========================================================================

	// Notification module subscribes to domain events (not HTTP-facing)
	notificationModule := notification.New(sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)

	// Initialize domain modules
	identityModule := identity.NewModule(pool, eventBus, storageSvc, cfg.GetMinioBucketOrganizationLogos(), val)
	authModule := auth.NewModule(pool, identityModule.Service(), cfg, eventBus, log, val)
	leadsModule, err := leads.NewModule(pool, eventBus, storageSvc, val, cfg, log)
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}
	leadAssigner := adapters.NewAppointmentsLeadAssigner(leadsModule.ManagementService())
	appointmentsModule := appointments.NewModule(pool, val, leadAssigner, sender)

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

	// Anti-Corruption Layer: Create adapter for cross-domain communication
	// This ensures leads module only depends on its own AgentProvider interface
	_ = adapters.NewAuthAgentProvider(authModule.Service())

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
