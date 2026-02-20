package router

import (
	"context"
	"net/http"
	"strings"
	"time"

	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/config"
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

	engine.Use(webhookCorsBypass())
	engine.Use(cors.New(buildCorsConfig(cfg)))

	// Security headers
	engine.Use(httpkit.SecurityHeaders())

	// Request logging
	engine.Use(httpkit.RequestLogger(log))

	// Global rate limiter (100 requests per second, burst of 200)
	globalLimiter := httpkit.NewIPRateLimiter(rate.Limit(100), 200, log)
	engine.Use(globalLimiter.RateLimit())

	registerHealthRoute(engine, app)

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

func webhookCorsBypass() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/webhook/") {
			origin := c.GetHeader("Origin")
			if origin != "" {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, X-Webhook-API-Key, X-Idempotency-Key")
				c.Header("Access-Control-Max-Age", "43200")
				c.Set("webhookOrigin", origin)
				c.Request.Header.Del("Origin")
			}
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

func buildCorsConfig(cfg config.HTTPConfig) cors.Config {
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Webhook-API-Key", "X-Idempotency-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: cfg.GetCORSAllowCreds(),
		MaxAge:           12 * time.Hour,
	}
	if cfg.GetCORSAllowAll() {
		corsConfig.AllowAllOrigins = true
		return corsConfig
	}
	corsConfig.AllowOrigins = cfg.GetCORSOrigins()
	return corsConfig
}

func registerHealthRoute(engine *gin.Engine, app *apphttp.App) {
	engine.GET("/api/health", func(c *gin.Context) {
		if app.Health != nil {
			timeoutCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := app.Health.Ping(timeoutCtx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
