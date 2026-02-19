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
	"portal_final_backend/internal/leads/maintenance"
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

	"github.com/google/uuid"
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

	worker.Run(ctx)
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
