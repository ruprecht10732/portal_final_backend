package router

import (
	"log"
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

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(cfg *config.Config, pool *pgxpool.Pool) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestTimer())

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
		log.Fatalf("failed to initialize email sender: %v", err)
	}

	// Auth module
	authRepo := authrepo.New(pool)
	authSvc := authservice.New(authRepo, cfg, sender)
	authHandler := authhandler.New(authSvc, cfg)

	// Leads module
	leadsRepo := leadsrepo.New(pool)
	leadsSvc := leadsservice.New(leadsRepo)
	leadsHandler := leadshandler.New(leadsSvc)

	engine.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	authHandler.RegisterRoutes(v1.Group("/auth"))

	// Protected routes
	protected := v1.Group("")
	protected.Use(middleware.AuthRequired(cfg))

	// Leads routes (accessible to all authenticated users)
	leadsHandler.RegisterRoutes(protected.Group("/leads"))

	// Admin routes
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthRequired(cfg), middleware.RequireRole("admin"))
	admin.PUT("/users/:id/roles", authHandler.SetUserRoles)

	return engine
}
