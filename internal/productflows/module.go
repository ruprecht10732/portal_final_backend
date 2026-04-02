package productflows

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/productflows/handler"
	"portal_final_backend/internal/productflows/repository"
	"portal_final_backend/internal/productflows/service"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check that Module implements http.Module.
var _ apphttp.Module = (*Module)(nil)

// Module is the product-flows bounded context module implementing http.Module.
type Module struct {
	handler *handler.Handler
	service *service.Service
	repo    repository.Repository
}

// NewModule creates and initializes the product-flows module.
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
	return "productflows"
}

// RegisterRoutes mounts product-flow routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Protected read-only endpoint for the offerte-wizard
	ctx.Protected.GET("/product-flows/:productGroupId", m.handler.GetFlow)

	// Admin CRUD endpoints for portal_final
	adminGroup := ctx.Admin.Group("/product-flows")
	adminGroup.GET("", m.handler.List)
	adminGroup.POST("", m.handler.Create)
	adminGroup.PUT("/:id", m.handler.Update)
	adminGroup.DELETE("/:id", m.handler.Delete)
	adminGroup.POST("/:id/duplicate", m.handler.Duplicate)
}
