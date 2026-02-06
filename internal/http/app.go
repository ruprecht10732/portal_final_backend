// Package http provides HTTP server infrastructure including module registration.
package http

import (
	"context"
	"portal_final_backend/internal/events"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
)

// RouterConfig combines the config interfaces needed by the HTTP router.
type RouterConfig interface {
	config.HTTPConfig
	config.JWTConfig
}

// HealthChecker exposes minimal functionality for readiness checks.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// App holds the fully initialized application dependencies.
// This is populated by main.go (the composition root) and passed to the router.
type App struct {
	// Config holds the router configuration (HTTP and JWT settings only).
	Config RouterConfig
	// Logger is the structured logger.
	Logger *logger.Logger
	// Health is used for readiness/health checks (e.g., DB ping).
	Health HealthChecker
	// EventBus is the domain event bus for cross-module communication.
	EventBus events.Bus
	// Modules contains all HTTP-facing domain modules.
	Modules []Module
}
