package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	debouncePrefix = "whatsappagent:debounce:"
	debounceTTL    = 4 * time.Second
	debounceWait   = 3 * time.Second
)

// MessageDebouncer batches rapid consecutive messages from the same phone
// so the agent receives all of them in a single run instead of replying to
// each message individually.
type MessageDebouncer struct {
	redis *redis.Client
	log   *logger.Logger
}

// NewMessageDebouncer creates a debouncer backed by Redis.
// When Redis is nil, debouncing is disabled and every message fires immediately.
func NewMessageDebouncer(client *redis.Client, log *logger.Logger) *MessageDebouncer {
	return &MessageDebouncer{redis: client, log: log}
}

// Claim sets a debounce nonce for the given phone key and returns that nonce.
// After debounceWait the caller should call ShouldProceed to check whether a
// newer message has superseded this one.
func (d *MessageDebouncer) Claim(ctx context.Context, phoneKey string) string {
	if d == nil || d.redis == nil {
		return ""
	}
	nonce := uuid.New().String()
	key := debouncePrefix + strings.TrimSpace(phoneKey)
	if err := d.redis.Set(ctx, key, nonce, debounceTTL).Err(); err != nil {
		if d.log != nil {
			d.log.WithContext(ctx).Warn("whatsappagent: debounce claim error; proceeding without debounce", "phone", phoneKey, "error", err)
		}
		return ""
	}
	return nonce
}

// Wait blocks for the debounce window. Call this between Claim and ShouldProceed.
func (d *MessageDebouncer) Wait() {
	if d == nil || d.redis == nil {
		return
	}
	time.Sleep(debounceWait)
}

// ShouldProceed returns true when the nonce is still current, meaning no newer
// message has arrived for this phone during the debounce window.
// An empty nonce (Redis disabled) always returns true.
func (d *MessageDebouncer) ShouldProceed(ctx context.Context, phoneKey, nonce string) bool {
	if d == nil || d.redis == nil || nonce == "" {
		return true
	}
	key := debouncePrefix + strings.TrimSpace(phoneKey)
	current, err := d.redis.Get(ctx, key).Result()
	if err != nil {
		if d.log != nil {
			d.log.WithContext(ctx).Warn("whatsappagent: debounce check error; proceeding", "phone", phoneKey, "error", err)
		}
		return true
	}
	if current != nonce {
		if d.log != nil {
			d.log.WithContext(ctx).Info(fmt.Sprintf("whatsappagent: debounce superseded; skipping reply"), "phone", phoneKey)
		}
		return false
	}
	return true
}
