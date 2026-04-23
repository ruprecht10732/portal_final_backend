// Package auth provides the authentication bounded context module.
// This file encapsulates all auth setup, dependency wiring, and route registration.
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

// AuthModuleConfig enforces the Interface Segregation Principle (ISP).
// The auth module only receives the exact configuration bounds it requires.
type AuthModuleConfig interface {
	config.AuthServiceConfig
	config.CookieConfig
	config.WebAuthnConfig
}

// Module implements apphttp.Module for the auth bounded context.
type Module struct {
	handler    *handler.Handler
	service    *service.Service
	repository *repository.Repository
}

// NewModule creates and initializes the auth module with all dependencies.
//
// Tech Debt (Security/Reliability): This constructor currently returns *Module,
// forcing it to swallow initialization errors. In v2, change this signature to
// `(*Module, error)` so the application can Fail-Fast on boot rather than
// starting in a partially broken, insecure state.
func NewModule(
	pool *pgxpool.Pool,
	identityService *identityservice.Service,
	cfg AuthModuleConfig,
	eventBus events.Bus,
	log *logger.Logger,
	val *validator.Validator,
) *Module {
	repo := repository.New(pool)
	svc := service.New(repo, identityService, cfg, eventBus, log)

	// Security: Do not fail silently. If WebAuthn fails to init, passkeys are dead.
	if err := svc.InitWebAuthn(cfg); err != nil {
		log.Error("CRITICAL: failed to initialize webauthn relying party", "error", err)
	}

	// Security: Ignoring validation registration errors can lead to unvalidated,
	// malicious payloads making it to the database.
	if err := authvalidator.RegisterAuthValidations(val); err != nil {
		log.Error("CRITICAL: failed to register auth custom validations", "error", err)
	}

	return &Module{
		handler:    handler.New(svc, cfg, val),
		service:    svc,
		repository: repo,
	}
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "auth"
}

// Service returns the concrete auth service.
// Tech Debt (Architecture): Other domains should depend on an Anti-Corruption Layer
// interface (like auth.UserProvider), not this concrete pointer.
func (m *Module) Service() *service.Service {
	return m.service
}

// Repository returns the concrete auth repository.
// Tech Debt (Architecture): Severe Bounded Context violation. Leaking the concrete
// repository allows other modules to bypass the service layer and execute raw queries.
func (m *Module) Repository() *repository.Repository {
	return m.repository
}

// RegisterRoutes mounts auth routes on the provided router context.
// Clean Code: Grouped by context and authorization level for O(1) human scanning.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	// ---------------------------------------------------------
	// Public Routes (Strictly Rate Limited)
	// ---------------------------------------------------------
	authGroup := ctx.V1.Group("/auth")
	authGroup.Use(ctx.AuthRateLimiter.RateLimit())
	m.handler.RegisterRoutes(authGroup) // Standard login/signup

	authGroup.POST("/passkey/login/begin", m.handler.BeginPasskeyLogin)
	authGroup.POST("/passkey/login/finish", m.handler.FinishPasskeyLogin)

	// ---------------------------------------------------------
	// Protected Routes (Requires Valid Session/Token)
	// ---------------------------------------------------------
	ctx.Protected.GET("/auth/verify", m.handler.Verify)

	// User Profile & Onboarding
	usersGroup := ctx.Protected.Group("/users")
	usersGroup.GET("/me", m.handler.GetMe)
	usersGroup.PATCH("/me", m.handler.UpdateMe)
	usersGroup.POST("/me/password", m.handler.ChangePassword)
	usersGroup.POST("/me/onboarding", m.handler.CompleteOnboarding)
	usersGroup.POST("/me/onboarding/complete", m.handler.MarkOnboardingComplete)

	// Passkey Management
	passkeysGroup := usersGroup.Group("/me/passkeys")
	passkeysGroup.GET("", m.handler.ListPasskeys)
	passkeysGroup.POST("/register/begin", m.handler.BeginPasskeyRegistration)
	passkeysGroup.POST("/register/finish", m.handler.FinishPasskeyRegistration)
	passkeysGroup.PATCH("/:credentialId", m.handler.RenamePasskey)
	passkeysGroup.DELETE("/:credentialId", m.handler.DeletePasskey)

	// Legacy / Broad scopes (See auth.go notes on pagination)
	usersGroup.GET("", m.handler.ListUsers)

	// ---------------------------------------------------------
	// Admin Routes
	// ---------------------------------------------------------
	ctx.Admin.PUT("/users/:id/roles", m.handler.SetUserRoles)
}

// Compile-time check to ensure Module implements the interface.
var _ apphttp.Module = (*Module)(nil)
