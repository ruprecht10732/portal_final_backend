package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	identityservice "portal_final_backend/internal/identity/service"
	leadrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/notification/outbox"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"

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

	notificationModule := notification.New(sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)
	notificationModule.SetWhatsAppSender(whatsapp.NewClient(cfg, log))
	notificationModule.SetLeadWhatsAppReader(leadrepo.New(pool))
	notificationModule.SetNotificationOutbox(outbox.New(pool))
	identityReader := identityrepo.New(pool)
	identitySvc := identityservice.New(identityReader, nil, nil, "", nil)
	notificationModule.SetOrganizationSettingsReader(identityReader)
	notificationModule.SetWorkflowResolver(identitySvc)

	dispatcher, err := scheduler.NewNotificationOutboxDispatcher(cfg, pool, log)
	if err != nil {
		log.Error("failed to initialize outbox dispatcher", "error", err)
		panic("failed to initialize outbox dispatcher: " + err.Error())
	}
	defer func() { _ = dispatcher.Close() }()
	go dispatcher.Run(ctx)

	worker, err := scheduler.NewWorker(cfg, pool, eventBus, log)
	if err != nil {
		log.Error("failed to initialize scheduler worker", "error", err)
		panic("failed to initialize scheduler worker: " + err.Error())
	}

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
