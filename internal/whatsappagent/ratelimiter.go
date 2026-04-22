package whatsappagent

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
	rateLimitPrefix = "whatsappagent:rate:"
	dedupePrefix    = "whatsappagent:dedupe:"
	dedupeTTL       = 10 * time.Minute
)

type RateLimiter struct {
	redis *redis.Client
	log   *logger.Logger
}

func NewRateLimiter(client *redis.Client, log *logger.Logger) *RateLimiter {
	if client == nil && log != nil {
		log.Warn("whatsappagent rate limiter running without redis; dedupe and throttling are disabled")
	}
	return &RateLimiter{redis: client, log: log}
}

func (r *RateLimiter) Allow(ctx context.Context, phone string) (bool, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return true, nil
	}
	if r.redis == nil {
		return true, nil
	}

	key := rateLimitPrefix + phone

	count, err := r.redis.Incr(ctx, key).Result()
	if err != nil {
		return true, fmt.Errorf("whatsappagent: rate limit incr error: %w", err)
	}

	if count == 1 {
		if err := r.redis.Expire(ctx, key, rateLimitWindow).Err(); err != nil {
			return true, fmt.Errorf("whatsappagent: rate limit expire error: %w", err)
		}
	}

	if count > rateLimitMax {
		if r.log != nil {
			r.log.Warn("whatsappagent: rate limit exceeded", "phone", phone, "count", count)
		}
		return false, nil
	}

	return true, nil
}

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
		return true, fmt.Errorf("whatsappagent: dedupe setnx error: %w", err)
	}
	return created, nil
}
