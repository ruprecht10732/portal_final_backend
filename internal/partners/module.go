// Package partners provides the partners bounded context module.
package partners

import (
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/partners/handler"
	"portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/partners/service"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the partners bounded context module implementing http.Module.
type Module struct {
	handler       *handler.Handler
	publicHandler *handler.PublicHandler
	service       *service.Service
}

// NewModule creates and initializes the partners module with all its dependencies.
func NewModule(
	pool *pgxpool.Pool,
	eventBus events.Bus,
	storageSvc storage.StorageService,
	logoBucket string,
	val *validator.Validator,
) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, eventBus, storageSvc, logoBucket)
	h := handler.New(svc, val)
	ph := handler.NewPublicHandler(svc, val)

	return &Module{handler: h, publicHandler: ph, service: svc}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "partners"
}

// Service returns the service layer for external use.
func (m *Module) Service() *service.Service {
	return m.service
}

// RegisterRoutes mounts partner routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	partnersGroup := ctx.Protected.Group("/partners")
	m.handler.RegisterRoutes(partnersGroup)

	// Public routes for vakman-facing offer pages (no auth middleware)
	publicGroup := ctx.V1.Group("/public/partner-offers")
	m.publicHandler.RegisterRoutes(publicGroup)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
