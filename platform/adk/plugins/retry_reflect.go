// Package plugins provides observability and resilience plugins for ADK agents.
package plugins

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RetryPolicy controls backoff behavior for retried tool calls.
type RetryPolicy struct {
	MaxAttempts  int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	RetryableErr func(error) bool
}

// DefaultRetryPolicy is a sensible default for external API calls.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    10 * time.Second,
		Multiplier:  2.0,
		RetryableErr: func(err error) bool {
			return err != nil // retry all non-nil errors by default
		},
	}
}

// contextKey is used for storing retry state in context.
type contextKey string

// ReflectErrorMessage extracts the last error from a retry-reflect context.
func ReflectErrorMessage(ctx context.Context) string {
	if v := ctx.Value(contextKey("retry_reflect_last_error")); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ReflectAttempt extracts the current attempt number from a retry-reflect context.
func ReflectAttempt(ctx context.Context) int {
	if v := ctx.Value(contextKey("retry_reflect_attempt")); v != nil {
		if n, ok := v.(int); ok {
			return n
		}
	}
	return 0
}

// WrapHandler wraps a tool handler function with automatic retry and structured
// error reflection. On failure, it serializes the error, feeds it back as
// context, and retries up to MaxAttempts with exponential backoff.
func WrapHandler[In any, Out any](base func(context.Context, In) (Out, error), policy RetryPolicy) func(context.Context, In) (Out, error) {
	return func(ctx context.Context, input In) (Out, error) {
		var zero Out
		var lastErr error
		delay := policy.BaseDelay

		for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
			result, err := base(ctx, input)
			lastErr = err

			if err == nil {
				return result, nil
			}

			if !policy.RetryableErr(err) {
				log.Printf("retry_reflect: handler non-retryable error on attempt %d/%d: %v",
					attempt, policy.MaxAttempts, err)
				return zero, err
			}

			if attempt == policy.MaxAttempts {
				log.Printf("retry_reflect: handler exhausted all %d attempts: %v",
					policy.MaxAttempts, err)
				break
			}

			log.Printf("retry_reflect: handler attempt %d/%d failed: %v. Retrying in %v...",
				attempt, policy.MaxAttempts, err, delay)

			ctx = context.WithValue(ctx, contextKey("retry_reflect_attempt"), attempt)
			ctx = context.WithValue(ctx, contextKey("retry_reflect_last_error"), err.Error())

			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * policy.Multiplier)
			if delay > policy.MaxDelay {
				delay = policy.MaxDelay
			}
		}

		return zero, fmt.Errorf("retry_reflect: handler failed after %d attempts: %w",
			policy.MaxAttempts, lastErr)
	}
}
