// Package auth provides the authentication bounded context module.
// This file defines the module that encapsulates all auth setup and route registration.
package auth

import (
	"portal_final_backend/internal/auth/handler"
	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/auth/service"
	authvalidator "portal_final_backend/internal/auth/validator"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	identityservice "portal_final_backend/internal/identity/service"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/validator"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthModuleConfig combines the config interfaces needed by the auth module.
// This ensures the module only receives the configuration it actually needs.
type AuthModuleConfig interface {
	config.AuthServiceConfig
	config.CookieConfig
	config.WebAuthnConfig
}

// Module is the auth bounded context module implementing http.Module.
type Module struct {
	handler    *handler.Handler
	service    *service.Service
	repository *repository.Repository
}

// NewModule creates and initializes the auth module with all its dependencies.
func NewModule(pool *pgxpool.Pool, identityService *identityservice.Service, cfg AuthModuleConfig, eventBus events.Bus, log *logger.Logger, val *validator.Validator) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, identityService, cfg, eventBus, log)

	// Initialize WebAuthn relying party
	if err := svc.InitWebAuthn(cfg); err != nil {
		log.Error("failed to initialize webauthn", "error", err)
	}

	// Register auth-specific validations on the injected validator
	_ = authvalidator.RegisterAuthValidations(val)

	h := handler.New(svc, cfg, val)

	return &Module{
		handler:    h,
		service:    svc,
		repository: repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "auth"
}

// Service returns the auth service for use by adapters (e.g., AgentProvider).
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the auth repository for use by adapters (e.g., agent email lookup).
func (m *Module) Repository() *repository.Repository {
	return m.repository
}

// RegisterRoutes mounts auth routes on the provided router context.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// Public auth routes with stricter rate limiting
	authGroup := ctx.V1.Group("/auth")
	authGroup.Use(ctx.AuthRateLimiter.RateLimit())
	m.handler.RegisterRoutes(authGroup)

	// Public passkey login (rate-limited)
	authGroup.POST("/passkey/login/begin", m.handler.BeginPasskeyLogin)
	authGroup.POST("/passkey/login/finish", m.handler.FinishPasskeyLogin)

	ctx.Protected.GET("/auth/verify", m.handler.Verify)

	// Protected user routes
	ctx.Protected.GET("/users/me", m.handler.GetMe)
	ctx.Protected.GET("/users", m.handler.ListUsers)
	ctx.Protected.PATCH("/users/me", m.handler.UpdateMe)
	ctx.Protected.POST("/users/me/password", m.handler.ChangePassword)
	ctx.Protected.POST("/users/me/onboarding", m.handler.CompleteOnboarding)
	ctx.Protected.POST("/users/me/onboarding/complete", m.handler.MarkOnboardingComplete)

	// Protected passkey management
	ctx.Protected.POST("/users/me/passkeys/register/begin", m.handler.BeginPasskeyRegistration)
	ctx.Protected.POST("/users/me/passkeys/register/finish", m.handler.FinishPasskeyRegistration)
	ctx.Protected.GET("/users/me/passkeys", m.handler.ListPasskeys)
	ctx.Protected.PATCH("/users/me/passkeys/:credentialId", m.handler.RenamePasskey)
	ctx.Protected.DELETE("/users/me/passkeys/:credentialId", m.handler.DeletePasskey)

	// Admin routes
	ctx.Admin.PUT("/users/:id/roles", m.handler.SetUserRoles)
}

// Compile-time check that Module implements http.Module
var _ apphttp.Module = (*Module)(nil)
