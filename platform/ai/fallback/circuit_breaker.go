package fallback

import (
	"sync"
	"time"
)

type circuitState int

const (
	stateClosed   circuitState = iota // healthy — requests pass through
	stateOpen                         // tripped — requests are rejected
	stateHalfOpen                     // probe — one request allowed to test recovery
)

// CircuitBreakerConfig configures the circuit breaker thresholds.
type CircuitBreakerConfig struct {
	FailureThreshold int           // consecutive failures before opening the circuit (default 3)
	ResetTimeout     time.Duration // how long to wait before probing (default 60s)
}

// CircuitBreaker implements a thread-safe circuit breaker state machine.
// States: closed -> open -> half-open -> closed.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            circuitState
	consecutiveFails int
	lastFailTime     time.Time
	failureThreshold int
	resetTimeout     time.Duration
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 60 * time.Second
	}
	return &CircuitBreaker{
		state:            stateClosed,
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
	}
}

// AllowRequest returns true if the primary should be attempted.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if time.Since(cb.lastFailTime) >= cb.resetTimeout {
			cb.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		return true
	}
	return true
}

// RecordSuccess resets the breaker to closed on a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFails = 0
	cb.state = stateClosed
}

// RecordFailure increments the failure counter and may trip the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case stateClosed:
		if cb.consecutiveFails >= cb.failureThreshold {
			cb.state = stateOpen
		}
	case stateHalfOpen:
		// Probe failed — reopen.
		cb.state = stateOpen
	}
}

// State returns the current state (for observability / tests).
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case stateClosed:
		return "closed"
	case stateOpen:
		if time.Since(cb.lastFailTime) >= cb.resetTimeout {
			return "half-open"
		}
		return "open"
	case stateHalfOpen:
		return "half-open"
	}
	return "unknown"
}
