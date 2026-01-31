package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/appointments"
	"portal_final_backend/internal/auth"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/http/router"
	"portal_final_backend/internal/leads"
	"portal_final_backend/internal/maps"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/services"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/db"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"
)

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

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
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

	// ========================================================================
	// Domain Modules (Composition Root)
	// ========================================================================

	// Notification module subscribes to domain events (not HTTP-facing)
	notificationModule := notification.New(sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)

	// Initialize domain modules
	authModule := auth.NewModule(pool, cfg, eventBus, log, val)
	leadsModule, err := leads.NewModule(pool, eventBus, val, cfg, log)
	if err != nil {
		log.Error("failed to initialize leads module", "error", err)
		panic("failed to initialize leads module: " + err.Error())
	}
	mapsModule := maps.NewModule(log)
	servicesModule := services.NewModule(pool, val, log)
	appointmentsModule := appointments.NewModule(pool, val)

	// Anti-Corruption Layer: Create adapter for cross-domain communication
	// This ensures leads module only depends on its own AgentProvider interface
	_ = adapters.NewAuthAgentProvider(authModule.Service())

	// ========================================================================
	// HTTP Layer
	// ========================================================================

	app := &apphttp.App{
		Config:   cfg,
		Logger:   log,
		EventBus: eventBus,
		Modules: []apphttp.Module{
			authModule,
			leadsModule,
			mapsModule,
			servicesModule,
			appointmentsModule,
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
