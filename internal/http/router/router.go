package router

import (
	"net/http"
	"time"

	"portal_final_backend/internal/adapters"
	"portal_final_backend/internal/auth"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/http/middleware"
	"portal_final_backend/internal/leads"
	"portal_final_backend/internal/logger"
	"portal_final_backend/internal/notification"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

func New(cfg *config.Config, pool *pgxpool.Pool, log *logger.Logger) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Security headers
	engine.Use(middleware.SecurityHeaders())

	// Request logging
	engine.Use(middleware.RequestLogger(log))

	// Global rate limiter (100 requests per second, burst of 200)
	globalLimiter := middleware.NewIPRateLimiter(rate.Limit(100), 200, log)
	engine.Use(globalLimiter.RateLimit())

	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: cfg.CORSAllowCreds,
		MaxAge:           12 * time.Hour,
	}
	if cfg.CORSAllowAll {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = cfg.CORSOrigins
	}
	engine.Use(cors.New(corsConfig))

	sender, err := email.NewSender(cfg)
	if err != nil {
		log.Error("failed to initialize email sender", "error", err)
		panic(err)
	}

	// Event bus for decoupled communication between modules
	eventBus := events.NewInMemoryBus(log)

	// Notification module subscribes to domain events (not HTTP-facing)
	notificationModule := notification.New(sender, cfg, log)
	notificationModule.RegisterHandlers(eventBus)

	// Health check endpoint (outside versioned API)
	engine.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Set up route groups
	v1 := engine.Group("/api/v1")
	protected := v1.Group("")
	protected.Use(middleware.AuthRequired(cfg))
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthRequired(cfg), middleware.RequireRole("admin"))

	// Router context provides shared dependencies to modules
	routerCtx := &apphttp.RouterContext{
		Engine:          engine,
		V1:              v1,
		Protected:       protected,
		Admin:           admin,
		Config:          cfg,
		AuthMiddleware:  middleware.AuthRequired(cfg),
		AuthRateLimiter: middleware.NewAuthRateLimiter(log),
	}

	// Initialize domain modules
	authModule := auth.NewModule(pool, cfg, eventBus, log)
	leadsModule := leads.NewModule(pool, eventBus)

	// Anti-Corruption Layer: Create adapter for cross-domain communication
	// This ensures leads module only depends on its own AgentProvider interface
	_ = adapters.NewAuthAgentProvider(authModule.Service())

	// Register all HTTP modules
	modules := []apphttp.Module{
		authModule,
		leadsModule,
	}

	for _, mod := range modules {
		log.Info("registering module routes", "module", mod.Name())
		mod.RegisterRoutes(routerCtx)
	}

	return engine
}
