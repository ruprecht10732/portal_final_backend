// Package db provides database connection infrastructure.
// This is part of the platform layer and contains no business logic.
package db

import (
	"context"
	"time"

	"portal_final_backend/platform/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new database connection pool with production-ready settings.
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.GetDatabaseURL())
	if err != nil {
		return nil, err
	}

	// Production-ready pool configuration
	poolConfig.MaxConns = 25                       // Maximum number of connections
	poolConfig.MinConns = 5                        // Minimum number of idle connections
	poolConfig.MaxConnLifetime = 1 * time.Hour     // Maximum connection lifetime
	poolConfig.MaxConnIdleTime = 30 * time.Minute  // Maximum idle time before closing
	poolConfig.HealthCheckPeriod = 1 * time.Minute // Health check interval

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
