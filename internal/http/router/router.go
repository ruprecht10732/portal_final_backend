package router

import (
	"net/http"
	"time"

	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// New creates a new Gin router with all middleware and module routes registered.
// The App struct contains all pre-initialized modules from the composition root (main.go).
// This keeps the router focused solely on HTTP concerns: middleware, routing, and CORS.
func New(app *apphttp.App) *gin.Engine {
	cfg := app.Config
	log := app.Logger

	engine := gin.New()
	engine.Use(gin.Recovery())

	// Security headers
	engine.Use(httpkit.SecurityHeaders())

	// Request logging
	engine.Use(httpkit.RequestLogger(log))

	// Global rate limiter (100 requests per second, burst of 200)
	globalLimiter := httpkit.NewIPRateLimiter(rate.Limit(100), 200, log)
	engine.Use(globalLimiter.RateLimit())

	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: cfg.GetCORSAllowCreds(),
		MaxAge:           12 * time.Hour,
	}
	if cfg.GetCORSAllowAll() {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = cfg.GetCORSOrigins()
	}
	engine.Use(cors.New(corsConfig))

	// Health check endpoint (outside versioned API)
	engine.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Set up route groups
	v1 := engine.Group("/api/v1")
	protected := v1.Group("")
	protected.Use(httpkit.AuthRequired(cfg))
	admin := v1.Group("/admin")
	admin.Use(httpkit.AuthRequired(cfg), httpkit.RequireRole("admin"))

	// Router context provides shared dependencies to modules
	routerCtx := &apphttp.RouterContext{
		Engine:          engine,
		V1:              v1,
		Protected:       protected,
		Admin:           admin,
		Config:          cfg,
		AuthMiddleware:  httpkit.AuthRequired(cfg),
		AuthRateLimiter: httpkit.NewAuthRateLimiter(log),
	}

	// Register all HTTP modules (already initialized by composition root)
	for _, mod := range app.Modules {
		log.Info("registering module routes", "module", mod.Name())
		mod.RegisterRoutes(routerCtx)
	}

	return engine
}
