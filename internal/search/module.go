package search

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/search/handler"
	"portal_final_backend/internal/search/repository"
	"portal_final_backend/internal/search/service"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.Handler
}

func NewModule(pool *pgxpool.Pool, val *validator.Validator) *Module {
	repo := repository.New(pool)
	svc := service.New(repo)
	h := handler.New(svc, val)

	return &Module{handler: h}
}

func (m *Module) Name() string {
	return "search"
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group("/search")
	m.handler.RegisterRoutes(group)
}

var _ apphttp.Module = (*Module)(nil)
