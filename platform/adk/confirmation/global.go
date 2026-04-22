package confirmation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

var (
	globalProvider     Provider
	globalProviderMu   sync.RWMutex
	globalRetryPolicy  struct {
		MaxAttempts  int
		BaseDelay    time.Duration
		MaxDelay     time.Duration
		Multiplier   float64
	}
)

// SetGlobalProvider sets the global confirmation provider used by all
// confirmation-wrapped tools.
func SetGlobalProvider(p Provider) {
	globalProviderMu.Lock()
	defer globalProviderMu.Unlock()
	globalProvider = p
}

// GetGlobalProvider returns the currently configured global confirmation provider.
func GetGlobalProvider() Provider {
	globalProviderMu.RLock()
	defer globalProviderMu.RUnlock()
	return globalProvider
}

// executeWithConfirmation handles the common confirmation logic for tool execution.
// It checks if confirmation is required, submits a request, polls for decision,
// and executes the base function if approved.
func executeWithConfirmation[Out any](
	ctx context.Context,
	toolName string,
	input any,
	provider Provider,
	base func() (Out, error),
) (Out, error) {
	var zero Out

	args := make(map[string]any)
	if b, err := json.Marshal(input); err == nil {
		_ = json.Unmarshal(b, &args)
	}

	need, reason, err := provider.RequiresConfirmation(ctx, toolName, args)
	if err != nil {
		return zero, fmt.Errorf("confirmation check failed for %s: %w", toolName, err)
	}
	if !need {
		return base()
	}

	req := Request{
		ID:          uuid.New(),
		AgentName:   "agent",
		ToolName:    toolName,
		Arguments:   args,
		Reason:      reason,
		RequestedAt: time.Now().UTC(),
		Decision:    DecisionPending,
	}
	if tenantID, ok := GetTenantID(ctx); ok {
		req.TenantID = tenantID
	}
	if err := provider.SubmitRequest(ctx, req); err != nil {
		return zero, fmt.Errorf("confirmation submission failed for %s: %w", toolName, err)
	}

	decision, err := provider.PollDecision(ctx, req.ID)
	if err != nil {
		return zero, fmt.Errorf("confirmation polling failed for %s: %w", toolName, err)
	}
	if decision != DecisionApproved {
		return zero, fmt.Errorf("tool %s %s by operator (reason: %s)", toolName, decision, reason)
	}

	return base()
}

// WrapToolHandler wraps a typed ADK tool handler with HITL confirmation logic.
// If the global provider determines confirmation is required, the wrapper
// submits a request to the database and blocks via PollDecision until
// approved or rejected before executing the underlying handler.
func WrapToolHandler[In any, Out any](toolName string, base func(tool.Context, In) (Out, error)) func(tool.Context, In) (Out, error) {
	return func(ctx tool.Context, input In) (Out, error) {
		provider := GetGlobalProvider()
		if provider == nil {
			return base(ctx, input)
		}

		return executeWithConfirmation(ctx, toolName, input, provider, func() (Out, error) {
			return base(ctx, input)
		})
	}
}

// WrapHandler wraps a typed context handler with HITL confirmation logic.
func WrapHandler[In any, Out any](toolName string, base func(context.Context, In) (Out, error)) func(context.Context, In) (Out, error) {
	return func(ctx context.Context, input In) (Out, error) {
		provider := GetGlobalProvider()
		if provider == nil {
			return base(ctx, input)
		}

		return executeWithConfirmation(ctx, toolName, input, provider, func() (Out, error) {
			return base(ctx, input)
		})
	}
}
