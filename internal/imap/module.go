package imap

import (
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	identityrepo "portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/imap/handler"
	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/imap/service"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.Handler
	service *service.Service
}

func NewModule(pool *pgxpool.Pool, val *validator.Validator, bus events.Bus, log *logger.Logger) *Module {
	repo := repository.New(pool)
	identityRepository := identityrepo.New(pool)
	svc := service.New(repo, identityRepository, bus, log)
	h := handler.New(svc, val)
	return &Module{
		handler: h,
		service: svc,
	}
}

func (m *Module) Name() string {
	return "imap"
}

func (m *Module) Service() *service.Service {
	return m.service
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group("/users/me/imap-accounts")
	m.handler.RegisterRoutes(group)
}

var _ apphttp.Module = (*Module)(nil)
