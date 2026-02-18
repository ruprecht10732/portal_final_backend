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

// SetEncryptionKey sets the AES key used for encrypting/decrypting export passwords.
func (m *Module) SetEncryptionKey(key []byte) {
	m.handler.SetEncryptionKey(key)
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "exports"
}

// RegisterRoutes mounts export routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	publicGroup := ctx.V1.Group("/exports")
	publicGroup.Use(BasicAuthMiddleware(m.repo))
	publicGroup.GET("/google-ads/conversions.csv", m.handler.ExportGoogleAdsCSV)

	adminGroup := ctx.Admin.Group("/exports/credentials")
	adminGroup.POST("", m.handler.HandleUpsertCredential)
	adminGroup.GET("", m.handler.HandleGetCredential)
	adminGroup.GET("/password", m.handler.HandleRevealPassword)
	adminGroup.DELETE("", m.handler.HandleDeleteCredential)
}

// Wait blocks until all background tasks in the exports module have completed.
// Call this during graceful server shutdown.
func (m *Module) Wait() { m.handler.Wait() }

var _ apphttp.Module = (*Module)(nil)
