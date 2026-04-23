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
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/qdrant"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Internal route path constants to resolve SonarLint S1192 and prevent routing typos.
const (
	pathVatRates        = "/catalog/vat-rates"
	pathProducts        = "/catalog/products"
	pathProductID       = "/:id"
	pathMaterials       = pathProductID + "/materials"
	pathAssets          = pathProductID + "/assets"
	pathAssetIDDownload = pathAssets + "/:assetId/download"
	pathAssetID         = pathAssets + "/:assetId"
)

// Module implements the apphttp.Module interface for the catalog domain.
type Module struct {
	handler *handler.Handler
	service *service.Service
	repo    repository.Repository
}

// NewModule initializes the catalog domain with its required adapters and services.
func NewModule(
	pool *pgxpool.Pool,
	storageSvc storage.StorageService,
	bucket string,
	val *validator.Validator,
	cfg *config.Config,
	log *logger.Logger,
) *Module {
	repo := repository.New(pool)

	// --- Semantic Search & AI Wiring ---
	var embedClient *embeddingapi.Client
	if cfg.IsCatalogEmbeddingEnabled() {
		embedClient = embeddingapi.NewClient(embeddingapi.Config{
			BaseURL:    cfg.GetCatalogEmbeddingAPIURL(),
			APIKey:     cfg.GetCatalogEmbeddingAPIKey(),
			Collection: cfg.GetCatalogEmbeddingCollection(),
		})
	}

	var searchEmbed *embeddings.Client
	if cfg.IsEmbeddingEnabled() {
		searchEmbed = embeddings.NewClient(embeddings.Config{
			BaseURL: cfg.GetEmbeddingAPIURL(),
			APIKey:  cfg.GetEmbeddingAPIKey(),
		})
	}

	// Factory helper to avoid repetitive Qdrant client boilerplate.
	newQdrant := func(collection string) *qdrant.Client {
		if cfg.GetQdrantURL() == "" || collection == "" {
			return nil
		}
		return qdrant.NewClient(qdrant.Config{
			BaseURL:    cfg.GetQdrantURL(),
			APIKey:     cfg.GetQdrantAPIKey(),
			Collection: collection,
		})
	}

	svc := service.New(service.Config{
		Repository:          repo,
		StorageService:      storageSvc,
		Bucket:              bucket,
		Logger:              log,
		EmbeddingClient:     embedClient,
		EmbeddingCollection: cfg.GetCatalogEmbeddingCollection(),
		SearchEmbedding:     searchEmbed,
		CatalogQdrant:       newQdrant(cfg.GetCatalogEmbeddingCollection()),
		QdrantClient:        newQdrant(cfg.GetQdrantCollection()),
		BouwmaatQdrant:      newQdrant(cfg.GetBouwmaatEmbeddingCollection()),
	})

	return &Module{
		repo:    repo,
		service: svc,
		handler: handler.New(svc, val),
	}
}

// Name returns the unique module identifier.
func (m *Module) Name() string {
	return "catalog"
}

// Service returns the domain service.
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the domain repository.
func (m *Module) Repository() repository.Repository {
	return m.repo
}

// RegisterRoutes mounts catalog routes using centralized path constants.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// ---------------------------------------------------------
	// VAT Rate Management
	// ---------------------------------------------------------
	vatProtected := ctx.Protected.Group(pathVatRates)
	{
		vatProtected.GET("", m.handler.ListVatRates)
		vatProtected.GET(pathProductID, m.handler.GetVatRateByID)
	}

	vatAdmin := ctx.Admin.Group(pathVatRates)
	{
		vatAdmin.POST("", m.handler.CreateVatRate)
		vatAdmin.PUT(pathProductID, m.handler.UpdateVatRate)
		vatAdmin.DELETE(pathProductID, m.handler.DeleteVatRate)
	}

	// ---------------------------------------------------------
	// Product Management
	// ---------------------------------------------------------
	prodProtected := ctx.Protected.Group(pathProducts)
	{
		prodProtected.GET("", m.handler.ListProducts)
		prodProtected.GET("/search", m.handler.SearchProductsForAutocomplete)
		prodProtected.GET(pathProductID, m.handler.GetProductByID)
		prodProtected.GET(pathMaterials, m.handler.ListProductMaterials)
		prodProtected.GET(pathAssets, m.handler.ListCatalogAssets)
		prodProtected.GET(pathAssetIDDownload, m.handler.GetCatalogAssetDownloadURL)
	}

	prodAdmin := ctx.Admin.Group(pathProducts)
	{
		prodAdmin.GET("/next-reference", m.handler.GetNextProductReference)
		prodAdmin.POST("", m.handler.CreateProduct)
		prodAdmin.PUT(pathProductID, m.handler.UpdateProduct)
		prodAdmin.DELETE(pathProductID, m.handler.DeleteProduct)

		// Materials
		prodAdmin.POST(pathMaterials, m.handler.AddProductMaterials)
		prodAdmin.DELETE(pathMaterials, m.handler.RemoveProductMaterials)

		// Assets
		prodAdmin.POST(pathProductID+"/assets/presign", m.handler.GetCatalogAssetPresign)
		prodAdmin.POST(pathAssets, m.handler.CreateCatalogAsset)
		prodAdmin.POST(pathProductID+"/assets/url", m.handler.CreateCatalogURLAsset)
		prodAdmin.DELETE(pathAssetID, m.handler.DeleteCatalogAsset)
	}
}

// RegisterHandlers subscribes the module to system-wide events.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {
	bus.Subscribe(events.OrganizationCreated{}.EventName(), m)
}

// Handle processes subscribed domain events.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	switch e := event.(type) {
	case events.OrganizationCreated:
		return m.service.SeedDefaultVatRates(ctx, e.OrganizationID)
	default:
		return nil
	}
}

// Compile-time check to ensure Module satisfies the router interface.
var _ apphttp.Module = (*Module)(nil)
