package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/catalog"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/leads"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/quotes"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic("failed to initialize email sender: " + err.Error())
	}

	notificationModule := notification.New(pool, sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)
	notificationModule.SetWhatsAppSender(whatsapp.NewClient(cfg, log))
	notificationModule.SetLeadWhatsAppReader(leadrepo.New(pool))
	notificationModule.SetOrganizationMemberReader(leadrepo.New(pool))
	notificationModule.SetNotificationOutbox(outbox.New(pool))
	identityReader := identityrepo.New(pool)
	identitySvc := identityservice.New(identityReader, nil, nil, "", nil)
	notificationModule.SetOrganizationSettingsReader(identityReader)
	notificationModule.SetWorkflowResolver(identitySvc)

	val := validator.New()

	// Worker-side quote generation wiring (no HTTP handlers required).
	catalogModule := catalog.NewModule(pool, nil, cfg.GetMinioBucketCatalogAssets(), val, cfg, log)
	leadsModule, err := leads.NewModule(pool, eventBus, nil, val, cfg, log)
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}
	quotesModule := quotes.NewModule(pool, eventBus, val)

	catalogReader := adapters.NewCatalogProductReader(catalogModule.Repository())
	leadsModule.SetCatalogReader(catalogReader)

	quotesDrafter := adapters.NewQuotesDraftWriter(quotesModule.Service())
	leadsModule.SetQuoteDrafter(quotesDrafter)

	quoteGenAdapter := adapters.NewQuoteGeneratorAdapter(leadsModule)
	quotesModule.Service().SetQuotePromptGenerator(quoteGenAdapter)

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

	worker, err := scheduler.NewWorker(cfg, pool, eventBus, log)
	if err != nil {
		log.Error("failed to initialize scheduler worker", "error", err)
		panic("failed to initialize scheduler worker: " + err.Error())
	}
	worker.SetQuoteJobProcessor(quotesModule.Service())

	worker.Run(ctx)
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
