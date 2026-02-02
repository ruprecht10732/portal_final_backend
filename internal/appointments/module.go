// Package appointments provides the appointments domain module.
package appointments

import (
	"portal_final_backend/internal/appointments/handler"
	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/email"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module represents the appointments domain module
type Module struct {
	handler *handler.Handler
	Service *service.Service
}

// NewModule creates a new appointments module with all dependencies wired
func NewModule(pool *pgxpool.Pool, val *validator.Validator, leadAssigner service.LeadAssigner, emailSender email.Sender) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, leadAssigner, emailSender)
	h := handler.New(svc, val)

	return &Module{
		handler: h,
		Service: svc,
	}
}

// Name returns the module name for logging
func (m *Module) Name() string {
	return "appointments"
}

// RegisterRoutes registers the module's routes under /api/appointments
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	appointments := ctx.Protected.Group("/appointments")
	m.handler.RegisterRoutes(appointments)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
