package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portal_final_backend/internal/config"
	"portal_final_backend/internal/db"
	"portal_final_backend/internal/http/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	engine := router.New(cfg, pool)

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- engine.Run(cfg.HTTPAddr)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = shutdownCtx
	case err := <-srvErr:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	}
}
