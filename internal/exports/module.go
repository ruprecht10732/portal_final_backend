package exports

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *Handler
	repo    *Repository
}

func NewModule(pool *pgxpool.Pool, val *validator.Validator) *Module {
	repo := NewRepository(pool)
	return &Module{
		handler: NewHandler(repo, val),
		repo:    repo,
	}
}

func (m *Module) SetEncryptionKey(key []byte) { m.handler.SetEncryptionKey(key) }
func (m *Module) Name() string                { return "exports" }

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	public := ctx.V1.Group("/exports")
	public.Use(BasicAuthMiddleware(m.repo))
	public.GET("/google-ads/conversions.csv", m.handler.ExportGoogleAdsCSV)

	admin := ctx.Admin.Group("/exports")
	{
		const path = "/credentials"
		admin.POST(path, m.handler.HandleUpsertCredential)
		admin.GET(path, m.handler.HandleGetCredential)
		admin.GET(path+"/password", m.handler.HandleRevealPassword)
		admin.DELETE(path, m.handler.HandleDeleteCredential)
	}
}

func (m *Module) Wait() { m.handler.Wait() }

var _ apphttp.Module = (*Module)(nil)
