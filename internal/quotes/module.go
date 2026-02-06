// Package quotes provides the quotes (offertes) domain module.
package quotes

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/quotes/handler"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/platform/events"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module represents the quotes domain module
type Module struct {
	handler       *handler.Handler
	publicHandler *handler.PublicHandler
	service       *service.Service
}

// NewModule creates a new quotes module with all dependencies wired
func NewModule(pool *pgxpool.Pool, eventBus *events.InMemoryBus, val *validator.Validator) *Module {
	repo := repository.New(pool)
	svc := service.New(repo)
	svc.SetEventBus(eventBus)
	h := handler.New(svc, val)
	ph := handler.NewPublicHandler(svc, val)

	return &Module{
		handler:       h,
		publicHandler: ph,
		service:       svc,
	}
}

// Name returns the module name for logging
func (m *Module) Name() string {
	return "quotes"
}

// Service returns the service layer for external use
func (m *Module) Service() *service.Service {
	return m.service
}

// RegisterRoutes registers the module's routes
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	quotes := ctx.Protected.Group("/quotes")
	m.handler.RegisterRoutes(quotes)

	// Public routes — no auth middleware
	publicQuotes := ctx.V1.Group("/public/quotes")
	m.publicHandler.RegisterRoutes(publicQuotes)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
