package router

import (
	"net/http"

	"portal_final_backend/internal/auth/handler"
	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(cfg *config.Config, pool *pgxpool.Pool) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestTimer())

	repo := repository.New(pool)
	svc := service.New(repo, cfg)
	apiHandler := handler.New(svc)

	engine.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	apiHandler.RegisterRoutes(v1.Group("/auth"))

	return engine
}
