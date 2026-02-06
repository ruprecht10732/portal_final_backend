// Package quotes provides the quotes (offertes) domain module.
package quotes

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/quotes/handler"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module represents the quotes domain module
type Module struct {
	handler *handler.Handler
	service *service.Service
}

// NewModule creates a new quotes module with all dependencies wired
func NewModule(pool *pgxpool.Pool, val *validator.Validator) *Module {
	repo := repository.New(pool)
	svc := service.New(repo)
	h := handler.New(svc, val)

	return &Module{
		handler: h,
		service: svc,
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
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
