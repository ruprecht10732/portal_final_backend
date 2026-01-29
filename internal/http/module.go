// Package http provides HTTP server infrastructure including the Module interface
// that all domain modules must implement for route registration.
package http

import (
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

// Module represents a bounded context that can register its HTTP routes.
// Each domain module implements this interface to encapsulate its own
// route setup, keeping the main router decoupled from specific endpoints.
type Module interface {
	// Name returns the module's identifier for logging purposes.
	Name() string
	// RegisterRoutes mounts the module's routes on the provided router group.
	// The RouterContext provides access to shared middleware and configuration.
	RegisterRoutes(ctx *RouterContext)
}

// RouterContext provides shared dependencies for module route registration.
// This avoids passing many parameters to each module's RegisterRoutes method.
type RouterContext struct {
	// Engine is the root Gin engine for modules that need engine-level access.
	Engine *gin.Engine
	// V1 is the /api/v1 route group.
	V1 *gin.RouterGroup
	// Protected is the authenticated route group under /api/v1.
	Protected *gin.RouterGroup
	// Admin is the admin-only route group under /api/v1/admin.
	Admin *gin.RouterGroup
	// Config is the JWT configuration for auth middleware (scoped access).
	Config config.JWTConfig
	// AuthMiddleware provides the authentication middleware.
	AuthMiddleware gin.HandlerFunc
	// AuthRateLimiter is the stricter rate limiter for auth routes.
	AuthRateLimiter *httpkit.AuthRateLimiter
}
