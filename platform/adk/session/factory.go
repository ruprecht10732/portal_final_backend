// Package session provides persistent ADK session backends.
package session

import (
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/adk/session"
)

// Config selects the session backend and its parameters.
type Config struct {
	// Backend is one of: "memory", "redis".
	Backend string
	// RedisClient is required when Backend == "redis".
	RedisClient *redis.Client
	// RedisPrefix is the key prefix for Redis entries.
	RedisPrefix string
	// RedisTTL is the session expiration time in Redis.
	RedisTTL time.Duration
}

// NewService creates a session.Service based on the configuration.
// Redis is the ONLY supported backend in production; calling this with an
// empty or invalid config will panic.
func NewService(cfg Config) session.Service {
	if cfg.Backend == "redis" && cfg.RedisClient != nil {
		return NewRedisService(cfg.RedisClient, cfg.RedisPrefix, cfg.RedisTTL)
	}
	panic("session.NewService: Redis backend is required. Set Backend=\"redis\" and provide a valid RedisClient.")
}
