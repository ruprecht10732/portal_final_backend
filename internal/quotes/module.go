// Package quotes provides the quotes (offertes) domain module.
package quotes

import (
	"portal_final_backend/internal/adapters/storage"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/quotes/handler"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/platform/events"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module represents the quotes domain module
type Module struct {
	handler       *handler.Handler
	publicHandler *handler.PublicHandler
	service       *service.Service
	repository    *repository.Repository
}

// NewModule creates a new quotes module with all dependencies wired
func NewModule(pool *pgxpool.Pool, eventBus *events.InMemoryBus, val *validator.Validator) *Module {
	repo := repository.New(pool)
	svc := service.New(repo)
	svc.SetEventBus(eventBus)
	h := handler.New(svc, val)
	ph := handler.NewPublicHandler(svc, val)

	return &Module{
		handler:       h,
		publicHandler: ph,
		service:       svc,
		repository:    repo,
	}
}

// Name returns the module name for logging
func (m *Module) Name() string {
	return "quotes"
}

// Service returns the service layer for external use
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the repository for use by adapters (e.g., PDF generation).
func (m *Module) Repository() *repository.Repository {
	return m.repository
}

// SetSSE injects the SSE service so public viewers get real-time quote updates.
func (m *Module) SetSSE(s *sse.Service) {
	m.publicHandler.SetSSE(s)
}

// SetStorageForPDF injects storage service for PDF download endpoints.
func (m *Module) SetStorageForPDF(svc storage.StorageService, bucket string) {
	m.handler.SetStorageForPDF(svc, bucket)
	m.publicHandler.SetStorageForPDF(svc, bucket)
}

// SetAttachmentBucket injects the bucket name for manual quote attachment uploads.
func (m *Module) SetAttachmentBucket(bucket string) {
	m.handler.SetAttachmentBucket(bucket)
}

// SetCatalogBucket injects the bucket name for catalog asset downloads via attachment preview.
func (m *Module) SetCatalogBucket(bucket string) {
	m.handler.SetCatalogBucket(bucket)
}

// SetPDFGenerator injects the on-demand PDF generator for lazy PDF creation in download endpoints.
func (m *Module) SetPDFGenerator(gen handler.PDFOnDemandGenerator) {
	m.handler.SetPDFGenerator(gen)
	m.publicHandler.SetPDFGenerator(gen)
}

// RegisterRoutes registers the module's routes
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	quotes := ctx.Protected.Group("/quotes")
	m.handler.RegisterRoutes(quotes)

	// Public routes — no auth middleware
	publicQuotes := ctx.V1.Group("/public/quotes")
	m.publicHandler.RegisterRoutes(publicQuotes)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
