// Package auth provides the authentication bounded context module.
// This file defines the module that encapsulates all auth setup and route registration.
package auth

import (
	"portal_final_backend/internal/auth/handler"
	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the auth bounded context module implementing http.Module.
type Module struct {
	handler *handler.Handler
	service *service.Service
	cfg     *config.Config
}

// NewModule creates and initializes the auth module with all its dependencies.
func NewModule(pool *pgxpool.Pool, cfg *config.Config, eventBus events.Bus, log *logger.Logger) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, cfg, eventBus, log)
	h := handler.New(svc, cfg)

	return &Module{
		handler: h,
		service: svc,
		cfg:     cfg,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "auth"
}

// Service returns the auth service for use by adapters (e.g., AgentProvider).
func (m *Module) Service() *service.Service {
	return m.service
}

// RegisterRoutes mounts auth routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Public auth routes with stricter rate limiting
	authGroup := ctx.V1.Group("/auth")
	authGroup.Use(ctx.AuthRateLimiter.RateLimit())
	m.handler.RegisterRoutes(authGroup)

	// Protected user routes
	ctx.Protected.GET("/users/me", m.handler.GetMe)
	ctx.Protected.GET("/users", m.handler.ListUsers)
	ctx.Protected.PATCH("/users/me", m.handler.UpdateMe)
	ctx.Protected.POST("/users/me/password", m.handler.ChangePassword)

	// Admin routes
	ctx.Admin.PUT("/users/:id/roles", m.handler.SetUserRoles)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
