// Package services provides the service types bounded context module.
// This module manages dynamic service categories that can be assigned to RAC_leads.
package services

import (
	"context"

	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/services/handler"
	"portal_final_backend/internal/services/repository"
	"portal_final_backend/internal/services/service"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the services bounded context module implementing http.Module.
type Module struct {
	handler *handler.Handler
	service *service.Service
	repo    repository.Repository
}

// NewModule creates and initializes the services module with all its dependencies.
func NewModule(pool *pgxpool.Pool, val *validator.Validator, log *logger.Logger) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, log)
	h := handler.New(svc, val)

	return &Module{
		handler: h,
		service: svc,
		repo:    repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "services"
}

// Service returns the service layer for external use.
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the repository for direct access if needed.
func (m *Module) Repository() repository.Repository {
	return m.repo
}

// RegisterRoutes mounts service type routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Protected read-only endpoints for active service types (tenant-scoped)
	ctx.Protected.GET("/service-types", m.handler.ListActive)
	ctx.Protected.GET("/service-types/:id", m.handler.GetByID)
	ctx.Protected.GET("/service-types/slug/:slug", m.handler.GetBySlug)

	// Admin-only CRUD endpoints
	adminGroup := ctx.Admin.Group("/service-types")
	adminGroup.GET("", m.handler.List)
	adminGroup.POST("", m.handler.Create)
	adminGroup.GET("/:id", m.handler.GetByID)
	adminGroup.PUT("/:id", m.handler.Update)
	adminGroup.DELETE("/:id", m.handler.Delete)
	adminGroup.PATCH("/:id/toggle-active", m.handler.ToggleActive)
}

// RegisterHandlers subscribes to domain events for seeding tenant defaults.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {
	bus.Subscribe(events.OrganizationCreated{}.EventName(), m)
}

// Handle routes events to the appropriate handler method.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	switch e := event.(type) {
	case events.OrganizationCreated:
		return m.service.SeedDefaults(ctx, e.OrganizationID)
	default:
		return nil
	}
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
