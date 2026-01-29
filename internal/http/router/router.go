package router

import (
	"net/http"
	"time"

	authhandler "portal_final_backend/internal/auth/handler"
	authrepo "portal_final_backend/internal/auth/repository"
	authservice "portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/http/middleware"
	leadshandler "portal_final_backend/internal/leads/handler"
	leadsrepo "portal_final_backend/internal/leads/repository"
	leadsservice "portal_final_backend/internal/leads/service"
	"portal_final_backend/internal/logger"

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

	// Auth module
	authRepo := authrepo.New(pool)
	authSvc := authservice.New(authRepo, cfg, sender, log)
	authHandler := authhandler.New(authSvc, cfg)

	// Leads module
	leadsRepo := leadsrepo.New(pool)
	leadsSvc := leadsservice.New(leadsRepo, sender)
	leadsHandler := leadshandler.New(leadsSvc)

	engine.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")

	// Auth routes with stricter rate limiting
	authGroup := v1.Group("/auth")
	authLimiter := middleware.NewAuthRateLimiter(log)
	authGroup.Use(authLimiter.RateLimit())
	authHandler.RegisterRoutes(authGroup)

	// Protected routes
	protected := v1.Group("")
	protected.Use(middleware.AuthRequired(cfg))
	protected.GET("/users/me", authHandler.GetMe)
	protected.GET("/users", authHandler.ListUsers)
	protected.PATCH("/users/me", authHandler.UpdateMe)
	protected.POST("/users/me/password", authHandler.ChangePassword)

	// Leads routes (accessible to all authenticated users)
	leadsHandler.RegisterRoutes(protected.Group("/leads"))

	// Admin routes
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthRequired(cfg), middleware.RequireRole("admin"))
	admin.PUT("/users/:id/roles", authHandler.SetUserRoles)

	return engine
}
