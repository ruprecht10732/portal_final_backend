package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	leadsdb "portal_final_backend/internal/leads/db"
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

	row, err := r.queries.CreateAIDecisionMemory(ctx, leadsdb.CreateAIDecisionMemoryParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		LeadID:         toPgUUIDPtr(params.LeadID),
		LeadServiceID:  toPgUUIDPtr(params.LeadServiceID),
		ServiceType:    params.ServiceType,
		DecisionType:   params.DecisionType,
		Outcome:        params.Outcome,
		Confidence:     toPgNumericPtr(params.Confidence),
		ContextSummary: params.ContextSummary,
		ActionSummary:  params.ActionSummary,
	})
	if err != nil {
		return AIDecisionMemory{}, err
	}

	return aiDecisionMemoryFromRow(row), nil
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
	var serviceTypeFilter *string
	if serviceType != "" {
		serviceTypeFilter = &serviceType
	}

	rows, err := r.queries.ListRecentAIDecisionMemories(ctx, leadsdb.ListRecentAIDecisionMemoriesParams{
		OrganizationID: toPgUUID(organizationID),
		Limit:          int32(limit),
		ServiceType:    toPgText(serviceTypeFilter),
	})
	if err != nil {
		return nil, fmt.Errorf("query ai decision memories: %w", err)
	}

	items := make([]AIDecisionMemory, 0, limit)
	for _, row := range rows {
		items = append(items, aiDecisionMemoryFromRow(row))
	}

	return items, nil
}

func aiDecisionMemoryFromRow(row leadsdb.RacAiDecisionMemory) AIDecisionMemory {
	return AIDecisionMemory{
		ID:             row.ID.Bytes,
		OrganizationID: row.OrganizationID.Bytes,
		LeadID:         optionalUUID(row.LeadID),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		ServiceType:    row.ServiceType,
		DecisionType:   row.DecisionType,
		Outcome:        row.Outcome,
		Confidence:     optionalNumericFloat64(row.Confidence),
		ContextSummary: row.ContextSummary,
		ActionSummary:  row.ActionSummary,
		CreatedAt:      row.CreatedAt.Time,
	}
}
