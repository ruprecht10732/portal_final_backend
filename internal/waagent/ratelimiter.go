package waagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/platform/logger"

	"github.com/redis/go-redis/v9"
)

const (
	rateLimitWindow = 5 * time.Minute
	rateLimitMax    = 30
	rateLimitPrefix = "waagent:rate:"
	dedupePrefix    = "waagent:dedupe:"
	dedupeTTL       = 10 * time.Minute
)

// RateLimiter enforces a sliding window rate limit per phone number using Redis.
type RateLimiter struct {
	redis *redis.Client
	log   *logger.Logger
}

// NewRateLimiter creates a new Redis-backed rate limiter.
func NewRateLimiter(client *redis.Client, log *logger.Logger) *RateLimiter {
	if client == nil && log != nil {
		log.Warn("waagent rate limiter running without redis; dedupe and throttling are disabled")
	}
	return &RateLimiter{redis: client, log: log}
}

// Allow returns true if the phone number is within the rate limit (30 calls per 5 minutes).
func (r *RateLimiter) Allow(ctx context.Context, phone string) (bool, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return true, nil
	}
	if r.redis == nil {
		return true, nil
	}

	key := rateLimitPrefix + phone

	pipe := r.redis.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rateLimitWindow)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return true, fmt.Errorf("waagent: rate limit pipeline error: %w", err)
	}

	count := incrCmd.Val()
	if count > rateLimitMax {
		if r.log != nil {
			r.log.Warn("waagent: rate limit exceeded", "phone", phone, "count", count)
		}
		return false, nil
	}

	return true, nil
}

// ClaimMessage returns true when this message ID has not been seen recently.
func (r *RateLimiter) ClaimMessage(ctx context.Context, messageID string) (bool, error) {
	messageID = strings.TrimSpace(messageID)
	if r.redis == nil {
		return true, nil
	}
	if messageID == "" {
		return true, nil
	}
	created, err := r.redis.SetNX(ctx, dedupePrefix+messageID, "1", dedupeTTL).Result()
	if err != nil {
		return true, fmt.Errorf("waagent: dedupe setnx error: %w", err)
	}
	return created, nil
}
