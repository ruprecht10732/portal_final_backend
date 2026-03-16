package whatsappagent

import (
	"context"
	"testing"

	"portal_final_backend/platform/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRateLimiterAllowsUntilWindowLimit(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client, logger.New("development"))
	ctx := context.Background()

	for i := 0; i < rateLimitMax; i++ {
		allowed, allowErr := limiter.Allow(ctx, " +31612345678 ")
		if allowErr != nil {
			t.Fatalf("allow returned error on iteration %d: %v", i, allowErr)
		}
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	allowed, allowErr := limiter.Allow(ctx, "+31612345678")
	if allowErr != nil {
		t.Fatalf("expected limit check without error, got %v", allowErr)
	}
	if allowed {
		t.Fatal("expected request above threshold to be rejected")
	}
}

func TestRateLimiterClaimMessageDeduplicatesTrimmedMessageIDs(t *testing.T) {
	t.Parallel()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer redisServer.Close()

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = client.Close() }()

	limiter := NewRateLimiter(client, logger.New("development"))
	ctx := context.Background()

	claimed, claimErr := limiter.ClaimMessage(ctx, "  msg-123  ")
	if claimErr != nil {
		t.Fatalf("expected first claim without error, got %v", claimErr)
	}
	if !claimed {
		t.Fatal("expected first message claim to succeed")
	}

	claimed, claimErr = limiter.ClaimMessage(ctx, "msg-123")
	if claimErr != nil {
		t.Fatalf("expected duplicate claim without error, got %v", claimErr)
	}
	if claimed {
		t.Fatal("expected duplicate message id to be rejected")
	}
}

func TestRateLimiterFailsOpenWithoutRedis(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(nil, logger.New("development"))
	ctx := context.Background()

	allowed, err := limiter.Allow(ctx, "")
	if err != nil {
		t.Fatalf("expected empty phone check without error, got %v", err)
	}
	if !allowed {
		t.Fatal("expected limiter without redis to allow requests")
	}

	claimed, err := limiter.ClaimMessage(ctx, "")
	if err != nil {
		t.Fatalf("expected empty message claim without error, got %v", err)
	}
	if !claimed {
		t.Fatal("expected limiter without redis to treat empty message ids as claimed")
	}

	claimed, err = limiter.ClaimMessage(ctx, "msg-123")
	if err != nil {
		t.Fatalf("expected redis-disabled dedupe without error, got %v", err)
	}
	if !claimed {
		t.Fatal("expected limiter without redis to fail open for dedupe")
	}
}
