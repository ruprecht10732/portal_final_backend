// Package confirmation provides Human-in-the-Loop (HITL) safeguards for
// high-stakes agent tool executions.
//
// NOTE: The ADK's functiontool.Config already supports RequireConfirmation and
// RequireConfirmationProvider natively. This package provides additional
// infrastructure for storing and polling approval requests in production.
package confirmation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Decision represents the outcome of a confirmation request.
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
	DecisionPending  Decision = "pending"
	DecisionExpired  Decision = "expired"
)

// Request captures a tool execution awaiting human approval.
type Request struct {
	ID          uuid.UUID
	AgentName   string
	ToolName    string
	Arguments   map[string]any
	Reason      string
	RequestedAt time.Time
	ExpiresAt   *time.Time
	Decision    Decision
	DecidedAt   *time.Time
	DecidedBy   *string
	TenantID    uuid.UUID
}

// tenantContextKey is the context key for propagating tenant ID into HITL wrappers.
type tenantContextKey struct{}

// WithTenantID returns a child context carrying the tenant ID for HITL lookups.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// GetTenantID extracts the tenant ID previously injected by WithTenantID.
func GetTenantID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantContextKey{}).(uuid.UUID)
	return id, ok
}

// Provider decides whether a tool execution requires human confirmation.
type Provider interface {
	// RequiresConfirmation evaluates the tool call and returns true if human
	// approval is required before execution. If true, reason explains why.
	RequiresConfirmation(ctx context.Context, toolName string, args map[string]any) (bool, string, error)
	// SubmitRequest stores a confirmation request and returns its ID.
	SubmitRequest(ctx context.Context, req Request) error
	// PollDecision checks the current decision for a confirmation request.
	PollDecision(ctx context.Context, id uuid.UUID) (Decision, error)
}

// ThresholdProvider is a concrete Provider that triggers confirmation based on
// configurable thresholds (e.g., quote value, pipeline stage risk).
type ThresholdProvider struct {
	// HighRiskTools is the set of tool names that always require confirmation.
	HighRiskTools map[string]struct{}
	// ThresholdEvaluators run custom logic per tool.
	ThresholdEvaluators map[string]func(args map[string]any) (bool, string)
}

// NewThresholdProvider creates a provider with sensible defaults for the
// portal backend.
func NewThresholdProvider() *ThresholdProvider {
	return &ThresholdProvider{
		HighRiskTools: map[string]struct{}{
			"UpdatePipelineStage": {},
			"DraftQuote":          {},
			"GenerateQuote":       {},
			"CreatePartnerOffer":  {},
			"CancelVisit":         {},
		},
		ThresholdEvaluators: make(map[string]func(args map[string]any) (bool, string)),
	}
}

// RegisterEvaluator adds a custom threshold check for a specific tool.
func (p *ThresholdProvider) RegisterEvaluator(toolName string, eval func(args map[string]any) (bool, string)) {
	p.ThresholdEvaluators[toolName] = eval
}

// RequiresConfirmation implements Provider.
func (p *ThresholdProvider) RequiresConfirmation(ctx context.Context, toolName string, args map[string]any) (bool, string, error) {
	if _, ok := p.HighRiskTools[toolName]; !ok {
		return false, "", nil
	}
	if eval, ok := p.ThresholdEvaluators[toolName]; ok {
		need, reason := eval(args)
		return need, reason, nil
	}
	return true, fmt.Sprintf("%s is a high-risk tool and requires approval", toolName), nil
}

// SubmitRequest implements Provider (no-op; override in storage-backed impl).
func (p *ThresholdProvider) SubmitRequest(ctx context.Context, req Request) error {
	return nil
}

// PollDecision implements Provider (auto-approve; override in storage-backed impl).
func (p *ThresholdProvider) PollDecision(ctx context.Context, id uuid.UUID) (Decision, error) {
	return DecisionApproved, nil
}

// Ensure ThresholdProvider implements Provider.
var _ Provider = (*ThresholdProvider)(nil)
