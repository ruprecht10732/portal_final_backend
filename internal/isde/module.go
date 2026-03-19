package isde

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/isde/handler"
	"portal_final_backend/internal/isde/repository"
	"portal_final_backend/internal/isde/service"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

const isdeRoutePath = "/isde"

// Module wires the ISDE bounded context.
type Module struct {
	handler *handler.Handler
	service *service.Service
	repo    repository.Repository
}

// NewModule creates a new ISDE module.
func NewModule(pool *pgxpool.Pool, val *validator.Validator, log *logger.Logger) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, log)
	h := handler.New(svc, val)

	return &Module{handler: h, service: svc, repo: repo}
}

// Name returns the module name.
func (m *Module) Name() string {
	return "isde"
}

// RegisterRoutes mounts ISDE routes.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group(isdeRoutePath)
	m.handler.RegisterRoutes(group)
}

// Service exposes the service for optional internal composition.
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository exposes the repository for optional internal composition.
func (m *Module) Repository() repository.Repository {
	return m.repo
}

var _ apphttp.Module = (*Module)(nil)
