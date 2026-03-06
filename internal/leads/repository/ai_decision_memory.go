package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIDecisionMemory is a compact historical record of an AI decision.
type AIDecisionMemory struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
	ServiceType    string
	DecisionType   string
	Outcome        string
	Confidence     *float64
	ContextSummary string
	ActionSummary  string
	CreatedAt      time.Time
}

// CreateAIDecisionMemoryParams contains input for persisting a memory row.
type CreateAIDecisionMemoryParams struct {
	OrganizationID uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
	ServiceType    string
	DecisionType   string
	Outcome        string
	Confidence     *float64
	ContextSummary string
	ActionSummary  string
}

func (r *Repository) CreateAIDecisionMemory(ctx context.Context, params CreateAIDecisionMemoryParams) (AIDecisionMemory, error) {
	if params.OrganizationID == uuid.Nil {
		return AIDecisionMemory{}, fmt.Errorf("organization_id is required")
	}
	params.ServiceType = strings.TrimSpace(params.ServiceType)
	if params.ServiceType == "" {
		return AIDecisionMemory{}, fmt.Errorf("service_type is required")
	}
	params.DecisionType = strings.TrimSpace(params.DecisionType)
	if params.DecisionType == "" {
		return AIDecisionMemory{}, fmt.Errorf("decision_type is required")
	}
	params.Outcome = strings.TrimSpace(params.Outcome)
	if params.Outcome == "" {
		return AIDecisionMemory{}, fmt.Errorf("outcome is required")
	}
	params.ContextSummary = strings.TrimSpace(params.ContextSummary)
	if params.ContextSummary == "" {
		return AIDecisionMemory{}, fmt.Errorf("context_summary is required")
	}
	params.ActionSummary = strings.TrimSpace(params.ActionSummary)
	if params.ActionSummary == "" {
		return AIDecisionMemory{}, fmt.Errorf("action_summary is required")
	}

	var item AIDecisionMemory
	err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_ai_decision_memory (
			organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
			confidence, context_summary, action_summary
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
		          confidence, context_summary, action_summary, created_at
	`,
		params.OrganizationID,
		params.LeadID,
		params.LeadServiceID,
		params.ServiceType,
		params.DecisionType,
		params.Outcome,
		params.Confidence,
		params.ContextSummary,
		params.ActionSummary,
	).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.LeadID,
		&item.LeadServiceID,
		&item.ServiceType,
		&item.DecisionType,
		&item.Outcome,
		&item.Confidence,
		&item.ContextSummary,
		&item.ActionSummary,
		&item.CreatedAt,
	)
	if err != nil {
		return AIDecisionMemory{}, err
	}

	return item, nil
}

func (r *Repository) ListRecentAIDecisionMemories(ctx context.Context, organizationID uuid.UUID, serviceType string, limit int) ([]AIDecisionMemory, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if limit <= 0 {
		limit = 6
	}
	if limit > 50 {
		limit = 50
	}

	serviceType = strings.TrimSpace(serviceType)

	// Build dynamic WHERE clause so Postgres can use optimal indexes.
	args := []any{organizationID}
	serviceTypeClause := ""
	argIdx := 2
	if serviceType != "" {
		serviceTypeClause = fmt.Sprintf(" AND service_type = $%d", argIdx)
		args = append(args, serviceType)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT id, organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
		       confidence, context_summary, action_summary, created_at
		FROM RAC_ai_decision_memory
		WHERE organization_id = $1%s
		ORDER BY created_at DESC
		LIMIT $%d
	`, serviceTypeClause, argIdx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ai decision memories: %w", err)
	}
	defer rows.Close()

	items := make([]AIDecisionMemory, 0, limit)
	for rows.Next() {
		var it AIDecisionMemory
		if err := rows.Scan(
			&it.ID,
			&it.OrganizationID,
			&it.LeadID,
			&it.LeadServiceID,
			&it.ServiceType,
			&it.DecisionType,
			&it.Outcome,
			&it.Confidence,
			&it.ContextSummary,
			&it.ActionSummary,
			&it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ai decision memory: %w", err)
		}
		items = append(items, it)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate ai decision memories: %w", rows.Err())
	}

	return items, nil
}
