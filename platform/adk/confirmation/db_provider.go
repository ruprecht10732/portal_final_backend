package confirmation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBProvider is a production-ready Provider that stores confirmation requests
// in PostgreSQL and blocks on PollDecision until a human responds.
type DBProvider struct {
	pool *pgxpool.Pool
	// PollInterval controls how often the database is polled for a decision.
	PollInterval time.Duration
	// DefaultTimeout is the maximum time to wait for a decision before auto-expiring.
	DefaultTimeout time.Duration
	base           *ThresholdProvider
}

// NewDBProvider creates a DB-backed confirmation provider.
func NewDBProvider(pool *pgxpool.Pool) *DBProvider {
	return &DBProvider{
		pool:           pool,
		PollInterval:   3 * time.Second,
		DefaultTimeout: 5 * time.Minute,
		base:           NewThresholdProvider(),
	}
}

// RegisterEvaluator delegates to the underlying ThresholdProvider.
func (p *DBProvider) RegisterEvaluator(toolName string, eval func(args map[string]any) (bool, string)) {
	p.base.RegisterEvaluator(toolName, eval)
}

// RequiresConfirmation delegates to the underlying ThresholdProvider.
func (p *DBProvider) RequiresConfirmation(ctx context.Context, toolName string, args map[string]any) (bool, string, error) {
	return p.base.RequiresConfirmation(ctx, toolName, args)
}

// SubmitRequest inserts a confirmation request into the agent_approvals table.
func (p *DBProvider) SubmitRequest(ctx context.Context, req Request) error {
	var expiresAt *time.Time
	if p.DefaultTimeout > 0 {
		t := req.RequestedAt.Add(p.DefaultTimeout)
		expiresAt = &t
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agent_approvals (id, agent_name, tool_name, arguments_json, reason, requested_at, expires_at, decision, lead_id, service_id, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, req.ID, req.AgentName, req.ToolName, mustMarshal(req.Arguments), req.Reason, req.RequestedAt, expiresAt, string(DecisionPending), nil, nil, req.TenantID)
	return err
}

// PollDecision blocks until the confirmation request reaches a terminal state
// (approved, rejected, or expired).
func (p *DBProvider) PollDecision(ctx context.Context, id uuid.UUID) (Decision, error) {
	deadline, hasDeadline := ctx.Deadline()
	ticker := time.NewTicker(p.PollInterval)
	defer ticker.Stop()

	for {
		var decision string
		var expiresAt *time.Time
		err := p.pool.QueryRow(ctx, `
			SELECT decision, expires_at FROM agent_approvals WHERE id = $1
		`, id).Scan(&decision, &expiresAt)
		if err != nil {
			return DecisionPending, fmt.Errorf("hitl: poll decision failed: %w", err)
		}

		if decision != string(DecisionPending) {
			return Decision(decision), nil
		}

		if expiresAt != nil && time.Now().After(*expiresAt) {
			_, _ = p.pool.Exec(ctx, `UPDATE agent_approvals SET decision = $1 WHERE id = $2`, string(DecisionExpired), id)
			return DecisionExpired, nil
		}

		select {
		case <-ctx.Done():
			return DecisionPending, ctx.Err()
		case <-ticker.C:
			if hasDeadline && time.Now().After(deadline) {
				return DecisionPending, fmt.Errorf("hitl: poll deadline exceeded")
			}
		}
	}
}

func mustMarshal(v map[string]any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// Ensure DBProvider implements Provider.
var _ Provider = (*DBProvider)(nil)
