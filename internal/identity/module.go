// Package identity provides the identity bounded context module.
package identity

import (
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/identity/handler"
	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.Handler
	service *service.Service
}

func NewModule(pool *pgxpool.Pool, eventBus events.Bus, storageSvc storage.StorageService, logoBucket string, val *validator.Validator, whatsappClient *whatsapp.Client) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, eventBus, storageSvc, logoBucket, whatsappClient)
	h := handler.New(svc, val)

	return &Module{handler: h, service: svc}
}

func (m *Module) Name() string {
	return "identity"
}

func (m *Module) Service() *service.Service {
	return m.service
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	m.handler.RegisterRoutes(ctx.Admin)
}

var _ apphttp.Module = (*Module)(nil)
