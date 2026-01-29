// Package leads provides the lead management bounded context module.
// This file defines the module that encapsulates all leads setup and route registration.
package leads

import (
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/leads/handler"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the leads bounded context module implementing http.Module.
type Module struct {
	handler *handler.Handler
	service *service.Service
}

// NewModule creates and initializes the leads module with all its dependencies.
func NewModule(pool *pgxpool.Pool, eventBus events.Bus) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, eventBus)
	h := handler.New(svc)

	return &Module{
		handler: h,
		service: svc,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "leads"
}

// Service returns the leads service for external use if needed.
func (m *Module) Service() *service.Service {
	return m.service
}

// RegisterRoutes mounts leads routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// All leads routes require authentication
	leadsGroup := ctx.Protected.Group("/leads")
	m.handler.RegisterRoutes(leadsGroup)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
