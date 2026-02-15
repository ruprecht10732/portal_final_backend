// Package catalog provides the catalog bounded context module.
package catalog

import (
	"context"
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/catalog/handler"
	"portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/catalog/service"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/ai/embeddingapi"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the catalog bounded context module implementing http.Module.
type Module struct {
	handler *handler.Handler
	service *service.Service
	repo    repository.Repository
}

// NewModule creates and initializes the catalog module.
func NewModule(pool *pgxpool.Pool, storageSvc storage.StorageService, bucket string, val *validator.Validator, cfg *config.Config, log *logger.Logger) *Module {
	repo := repository.New(pool)

	var embedClient *embeddingapi.Client
	if cfg.IsCatalogEmbeddingEnabled() {
		embedClient = embeddingapi.NewClient(embeddingapi.Config{
			BaseURL:    cfg.GetCatalogEmbeddingAPIURL(),
			APIKey:     cfg.GetCatalogEmbeddingAPIKey(),
			Collection: cfg.GetCatalogEmbeddingCollection(),
		})
	}

	svc := service.New(repo, storageSvc, bucket, log, embedClient, cfg.GetCatalogEmbeddingCollection())
	h := handler.New(svc, val)

	return &Module{
		handler: h,
		service: svc,
		repo:    repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "catalog"
}

// Service returns the service layer for external use.
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the repository for direct access if needed.
func (m *Module) Repository() repository.Repository {
	return m.repo
}

// RegisterRoutes mounts catalog routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Protected read-only endpoints
	ctx.Protected.GET("/catalog/vat-rates", m.handler.ListVatRates)
	ctx.Protected.GET("/catalog/vat-rates/:id", m.handler.GetVatRateByID)
	ctx.Protected.GET("/catalog/products", m.handler.ListProducts)
	ctx.Protected.GET("/catalog/products/:id", m.handler.GetProductByID)
	ctx.Protected.GET("/catalog/products/search", m.handler.SearchProductsForAutocomplete)
	ctx.Protected.GET("/catalog/products/:id/materials", m.handler.ListProductMaterials)
	ctx.Protected.GET("/catalog/products/:id/assets", m.handler.ListCatalogAssets)
	ctx.Protected.GET("/catalog/products/:id/assets/:assetId/download", m.handler.GetCatalogAssetDownloadURL)

	// Admin CRUD endpoints
	adminGroup := ctx.Admin.Group("/catalog")
	adminGroup.POST("/vat-rates", m.handler.CreateVatRate)
	adminGroup.PUT("/vat-rates/:id", m.handler.UpdateVatRate)
	adminGroup.DELETE("/vat-rates/:id", m.handler.DeleteVatRate)

	adminGroup.GET("/products/next-reference", m.handler.GetNextProductReference)
	adminGroup.POST("/products", m.handler.CreateProduct)
	adminGroup.PUT("/products/:id", m.handler.UpdateProduct)
	adminGroup.DELETE("/products/:id", m.handler.DeleteProduct)
	adminGroup.POST("/products/:id/materials", m.handler.AddProductMaterials)
	adminGroup.DELETE("/products/:id/materials", m.handler.RemoveProductMaterials)
	adminGroup.POST("/products/:id/assets/presign", m.handler.GetCatalogAssetPresign)
	adminGroup.POST("/products/:id/assets", m.handler.CreateCatalogAsset)
	adminGroup.POST("/products/:id/assets/url", m.handler.CreateCatalogURLAsset)
	adminGroup.DELETE("/products/:id/assets/:assetId", m.handler.DeleteCatalogAsset)
}

// RegisterHandlers subscribes to domain events for seeding tenant defaults.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {
	bus.Subscribe(events.OrganizationCreated{}.EventName(), m)
}

// Handle routes events to the appropriate handler method.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	switch e := event.(type) {
	case events.OrganizationCreated:
		return m.service.SeedDefaultVatRates(ctx, e.OrganizationID)
	default:
		return nil
	}
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
