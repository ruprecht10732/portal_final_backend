package exports

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the exports bounded context module implementing http.Module.
type Module struct {
	handler *Handler
	repo    *Repository
}

// NewModule creates and initializes the exports module.
func NewModule(pool *pgxpool.Pool, val *validator.Validator) *Module {
	repo := NewRepository(pool)
	handler := NewHandler(repo, val)

	return &Module{
		handler: handler,
		repo:    repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "exports"
}

// RegisterRoutes mounts export routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	publicGroup := ctx.V1.Group("/exports")
	publicGroup.Use(APIKeyAuthMiddleware(m.repo))
	publicGroup.GET("/google-ads/conversions.csv", m.handler.ExportGoogleAdsCSV)

	adminGroup := ctx.Admin.Group("/exports/keys")
	adminGroup.POST("", m.handler.HandleCreateAPIKey)
	adminGroup.GET("", m.handler.HandleListAPIKeys)
	adminGroup.DELETE("/:keyId", m.handler.HandleRevokeAPIKey)
}

var _ apphttp.Module = (*Module)(nil)
