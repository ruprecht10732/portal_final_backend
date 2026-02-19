// Package webhook provides the webhook/form capture bounded context module.
// This file defines the module that encapsulates all webhook setup and route registration.
package webhook

import (
	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the webhook bounded context module implementing http.Module.
type Module struct {
	handler *Handler
	repo    *Repository
}

// NewModule creates and initializes the webhook module with all its dependencies.
func NewModule(pool *pgxpool.Pool, leadCreator LeadCreator, storageSvc storage.StorageService, storageBucket string, eventBus events.Bus, val *validator.Validator, log *logger.Logger) *Module {
	repo := NewRepository(pool)
	service := NewService(repo, leadCreator, storageSvc, storageBucket, eventBus, log)
	handler := NewHandler(service, repo, val)

	return &Module{
		handler: handler,
		repo:    repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "webhook"
}

// RegisterRoutes mounts webhook routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Public webhook endpoint (API key auth, no JWT)
	webhookGroup := ctx.V1.Group("/webhook")
	webhookGroup.Use(APIKeyAuthMiddleware(m.repo))
	webhookGroup.POST("/forms", m.handler.HandleFormSubmission)
	webhookGroup.GET("/config", m.handler.HandleGetWebhookConfig)

	// Public Google Lead Form webhook (payload auth)
	ctx.V1.POST("/webhook/google-leads", m.handler.HandleGoogleLeadWebhook)

	// Admin API key management (JWT auth + admin role)
	adminGroup := ctx.Admin.Group("/webhook/keys")
	adminGroup.POST("", m.handler.HandleCreateAPIKey)
	adminGroup.GET("", m.handler.HandleListAPIKeys)
	adminGroup.DELETE("/:keyId", m.handler.HandleRevokeAPIKey)

	// Admin GTM config management (JWT auth + admin role)
	gtmAdmin := ctx.Admin.Group("/webhook/gtm-config")
	gtmAdmin.GET("", m.handler.HandleGetGTMConfig)
	gtmAdmin.PUT("", m.handler.HandleUpdateGTMConfig)
	gtmAdmin.DELETE("", m.handler.HandleDeleteGTMConfig)

	// Admin Google webhook config management
	googleAdmin := ctx.Admin.Group("/webhook/google-lead-config")
	googleAdmin.POST("", m.handler.HandleCreateGoogleWebhookConfig)
	googleAdmin.GET("", m.handler.HandleListGoogleWebhookConfigs)
	googleAdmin.PUT("/:configId/campaigns", m.handler.HandleUpdateGoogleCampaignMapping)
	googleAdmin.DELETE("/:configId/campaigns/:campaignId", m.handler.HandleDeleteGoogleCampaignMapping)
	googleAdmin.DELETE("/:configId", m.handler.HandleDeleteGoogleWebhookConfig)

	// SDK serving (public, no auth)
	ctx.V1.GET("/webhook/sdk.js", m.handler.HandleServeSDK)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
