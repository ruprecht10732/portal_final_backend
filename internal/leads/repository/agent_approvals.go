package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

// AgentApproval represents a human-in-the-loop confirmation request.
type AgentApproval struct {
	ID            uuid.UUID
	AgentName     string
	ToolName      string
	Arguments     map[string]any
	Reason        string
	RequestedAt   time.Time
	ExpiresAt     *time.Time
	Decision      string
	DecidedAt     *time.Time
	DecidedBy     *string
	LeadID        *uuid.UUID
	ServiceID     *uuid.UUID
	TenantID      uuid.UUID
	CreatedAt     time.Time
}

// CreateAgentApprovalParams is the input for creating an approval request.
type CreateAgentApprovalParams struct {
	ID          uuid.UUID
	AgentName   string
	ToolName    string
	Arguments   map[string]any
	Reason      string
	RequestedAt time.Time
	ExpiresAt   *time.Time
	LeadID      *uuid.UUID
	ServiceID   *uuid.UUID
	TenantID    uuid.UUID
}

// ListAgentApprovalsParams filters for listing approvals.
type ListAgentApprovalsParams struct {
	TenantID uuid.UUID
	Status   string
	Limit    int32
	Offset   int32
}

// UpdateAgentApprovalDecisionParams is the input for resolving an approval.
type UpdateAgentApprovalDecisionParams struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Decision  string
	DecidedBy string
}

// CreateAgentApproval inserts a new approval request.
func (r *Repository) CreateAgentApproval(ctx context.Context, params CreateAgentApprovalParams) (AgentApproval, error) {
	argsJSON, _ := json.Marshal(params.Arguments)
	expiresAt := pgtype.Timestamptz{Valid: false}
	if params.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *params.ExpiresAt, Valid: true}
	}
	leadID := pgtype.UUID{Valid: false}
	if params.LeadID != nil {
		leadID = pgtype.UUID{Bytes: *params.LeadID, Valid: true}
	}
	serviceID := pgtype.UUID{Valid: false}
	if params.ServiceID != nil {
		serviceID = pgtype.UUID{Bytes: *params.ServiceID, Valid: true}
	}
	row, err := r.queries.CreateAgentApproval(ctx, leadsdb.CreateAgentApprovalParams{
		ID:            pgtype.UUID{Bytes: params.ID, Valid: true},
		AgentName:     params.AgentName,
		ToolName:      params.ToolName,
		ArgumentsJson: argsJSON,
		Reason:        params.Reason,
		RequestedAt:   pgtype.Timestamptz{Time: params.RequestedAt, Valid: true},
		ExpiresAt:     expiresAt,
		Decision:      "pending",
		DecidedAt:     pgtype.Timestamptz{Valid: false},
		DecidedBy:     pgtype.Text{Valid: false},
		LeadID:        leadID,
		ServiceID:     serviceID,
		TenantID:      pgtype.UUID{Bytes: params.TenantID, Valid: true},
	})
	if err != nil {
		return AgentApproval{}, fmt.Errorf("create agent approval: %w", err)
	}
	return mapAgentApproval(row), nil
}

// ListPendingAgentApprovals returns pending approvals for a tenant.
func (r *Repository) ListPendingAgentApprovals(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]AgentApproval, error) {
	rows, err := r.queries.ListPendingAgentApprovals(ctx, leadsdb.ListPendingAgentApprovalsParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list pending approvals: %w", err)
	}
	approvals := make([]AgentApproval, 0, len(rows))
	for _, row := range rows {
		approvals = append(approvals, mapAgentApproval(row))
	}
	return approvals, nil
}

// GetAgentApprovalByID fetches a single approval by ID scoped to tenant.
func (r *Repository) GetAgentApprovalByID(ctx context.Context, id, tenantID uuid.UUID) (AgentApproval, error) {
	row, err := r.queries.GetAgentApprovalByID(ctx, leadsdb.GetAgentApprovalByIDParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return AgentApproval{}, fmt.Errorf("get agent approval: %w", err)
	}
	return mapAgentApproval(row), nil
}

// UpdateAgentApprovalDecision resolves a pending approval.
func (r *Repository) UpdateAgentApprovalDecision(ctx context.Context, params UpdateAgentApprovalDecisionParams) error {
	err := r.queries.UpdateAgentApprovalDecision(ctx, leadsdb.UpdateAgentApprovalDecisionParams{
		Decision:  params.Decision,
		DecidedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		DecidedBy: pgtype.Text{String: params.DecidedBy, Valid: true},
		ID:        pgtype.UUID{Bytes: params.ID, Valid: true},
		TenantID:  pgtype.UUID{Bytes: params.TenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update approval decision: %w", err)
	}
	return nil
}

// CountPendingAgentApprovals returns the number of pending approvals.
func (r *Repository) CountPendingAgentApprovals(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return r.queries.CountPendingAgentApprovals(ctx, pgtype.UUID{Bytes: tenantID, Valid: true})
}

func mapAgentApproval(row leadsdb.AgentApproval) AgentApproval {
	a := AgentApproval{
		ID:          uuid.UUID(row.ID.Bytes),
		AgentName:   row.AgentName,
		ToolName:    row.ToolName,
		Reason:      row.Reason,
		Decision:    row.Decision,
		RequestedAt: row.RequestedAt.Time,
		CreatedAt:   row.CreatedAt.Time,
	}
	if row.ArgumentsJson != nil {
		_ = json.Unmarshal(row.ArgumentsJson, &a.Arguments)
	}
	if row.ExpiresAt.Valid {
		a.ExpiresAt = &row.ExpiresAt.Time
	}
	if row.DecidedAt.Valid {
		a.DecidedAt = &row.DecidedAt.Time
	}
	if row.DecidedBy.Valid {
		a.DecidedBy = &row.DecidedBy.String
	}
	if row.LeadID.Valid {
		id := uuid.UUID(row.LeadID.Bytes)
		a.LeadID = &id
	}
	if row.ServiceID.Valid {
		id := uuid.UUID(row.ServiceID.Bytes)
		a.ServiceID = &id
	}
	if row.TenantID.Valid {
		a.TenantID = uuid.UUID(row.TenantID.Bytes)
	}
	return a
}
