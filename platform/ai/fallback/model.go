package fallback

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"math/rand/v2"
	"time"

	"google.golang.org/adk/model"
)

const (
	secondaryMaxRetries    = 4
	secondaryBaseBackoff   = 2 * time.Second
	secondaryBackoffFactor = 2.0
	jitterFraction         = 0.3
)

// Model wraps a primary and secondary model.LLM with circuit-breaker-based
// automatic failover and exponential backoff on the secondary.
type Model struct {
	primary   model.LLM
	secondary model.LLM
	breaker   *CircuitBreaker
	logger    *slog.Logger
}

// Config for constructing a fallback Model.
type Config struct {
	Primary              model.LLM
	Secondary            model.LLM
	CircuitBreakerConfig CircuitBreakerConfig
	Logger               *slog.Logger
}

// NewModel creates a fallback-aware LLM wrapper.
// If secondary is nil the wrapper behaves as a transparent pass-through.
func NewModel(cfg Config) *Model {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Model{
		primary:   cfg.Primary,
		secondary: cfg.Secondary,
		breaker:   NewCircuitBreaker(cfg.CircuitBreakerConfig),
		logger:    logger,
	}
}

func (m *Model) Name() string {
	return m.primary.Name()
}

// GenerateContent implements model.LLM. It tries the primary provider first
// (subject to circuit-breaker state), falling back to the secondary on error.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generateWithFallback(ctx, req, stream)
		yield(resp, err)
	}
}

func (m *Model) generateWithFallback(ctx context.Context, req *model.LLMRequest, stream bool) (*model.LLMResponse, error) {
	// Try primary if circuit allows it.
	if m.breaker.AllowRequest() {
		resp, err := m.callModel(ctx, m.primary, req, stream)
		if err == nil {
			m.breaker.RecordSuccess()
			return resp, nil
		}
		m.breaker.RecordFailure()
		m.logger.Warn("llm fallback: primary provider failed",
			"provider", m.primary.Name(),
			"error", err,
			"circuit_state", m.breaker.State(),
		)
	} else {
		m.logger.Info("llm fallback: circuit open, skipping primary",
			"provider", m.primary.Name(),
			"circuit_state", m.breaker.State(),
		)
	}

	// Fallback to secondary with exponential backoff + retries.
	if m.secondary == nil {
		return nil, fmt.Errorf("llm fallback: primary failed and no secondary configured")
	}

	var lastErr error
	for attempt := 0; attempt <= secondaryMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			m.logger.Info("llm fallback: retrying secondary",
				"provider", m.secondary.Name(),
				"attempt", attempt+1,
				"backoff", backoff,
			)
			if err := sleepWithContext(ctx, backoff); err != nil {
				return nil, fmt.Errorf("llm fallback: context cancelled during backoff: %w", err)
			}
		}

		resp, err := m.callModel(ctx, m.secondary, req, stream)
		if err == nil {
			m.logger.Info("llm fallback: secondary succeeded",
				"provider", m.secondary.Name(),
				"attempt", attempt+1,
			)
			return resp, nil
		}
		lastErr = err
		m.logger.Warn("llm fallback: secondary attempt failed",
			"provider", m.secondary.Name(),
			"attempt", attempt+1,
			"error", err,
		)
	}

	return nil, fmt.Errorf("llm fallback: all providers exhausted, last error: %w", lastErr)
}

// callModel calls a model.LLM and collects the first (non-streaming) response.
func (m *Model) callModel(ctx context.Context, llm model.LLM, req *model.LLMRequest, stream bool) (*model.LLMResponse, error) {
	for resp, err := range llm.GenerateContent(ctx, req, stream) {
		return resp, err
	}
	return nil, fmt.Errorf("llm fallback: model %s returned no response", llm.Name())
}

// backoffDuration returns the sleep duration for a given retry attempt (1-indexed).
func backoffDuration(attempt int) time.Duration {
	base := float64(secondaryBaseBackoff)
	for i := 1; i < attempt; i++ {
		base *= secondaryBackoffFactor
	}
	// Add jitter: ±30%
	jitter := base * jitterFraction * (2*rand.Float64() - 1)
	d := time.Duration(base + jitter)
	if d < 0 {
		d = secondaryBaseBackoff
	}
	return d
}

// sleepWithContext sleeps for the given duration but returns early if the context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
