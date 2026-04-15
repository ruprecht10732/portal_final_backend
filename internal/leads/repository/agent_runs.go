package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

// =====================================
// Agent Run Domain Types
// =====================================

type InsertAgentRunParams struct {
	LeadID        uuid.UUID
	ServiceID     uuid.UUID
	TenantID      uuid.UUID
	AgentName     string
	RunID         string
	SessionLabel  string
	ModelUsed     string
	ReasoningMode string
	StartedAt     time.Time
	DurationMs    int
	ToolCallCount int
	TokenInput    int
	TokenOutput   int
	Outcome       string
	OutcomeDetail string
	CycleCount    int
}

type CompleteAgentRunParams struct {
	ID            uuid.UUID
	DurationMs    *int
	ToolCallCount int
	TokenInput    int
	TokenOutput   int
	Outcome       string
	OutcomeDetail string
}

type InsertAgentToolCallParams struct {
	AgentRunID    uuid.UUID
	SequenceNum   int
	ToolName      string
	ArgumentsJSON []byte
	ResponseJSON  []byte
	HasError      bool
	ErrorMessage  string
	DurationMs    int
}

type AgentRun struct {
	ID            uuid.UUID
	LeadID        uuid.UUID
	ServiceID     uuid.UUID
	TenantID      uuid.UUID
	AgentName     string
	RunID         string
	SessionLabel  string
	ModelUsed     string
	ReasoningMode string
	StartedAt     time.Time
	FinishedAt    *time.Time
	DurationMs    *int
	ToolCallCount int
	TokenInput    int
	TokenOutput   int
	Outcome       string
	OutcomeDetail string
	CycleCount    int
	CreatedAt     time.Time
}

type AgentHealthStats struct {
	TotalRuns        int64
	SuccessCount     int64
	FallbackCount    int64
	EscalationCount  int64
	LoopCount        int64
	ErrorCount       int64
	TimeoutCount     int64
	AvgToolCalls     int
	AvgDurationMs    int
	TotalTokenInput  int64
	TotalTokenOutput int64
}

// =====================================
// Repository Methods
// =====================================

func (r *Repository) InsertAgentRun(ctx context.Context, params InsertAgentRunParams) (uuid.UUID, error) {
	row, err := r.queries.InsertAgentRun(ctx, leadsdb.InsertAgentRunParams{
		LeadID:        pgtype.UUID{Bytes: params.LeadID, Valid: true},
		ServiceID:     pgtype.UUID{Bytes: params.ServiceID, Valid: true},
		TenantID:      pgtype.UUID{Bytes: params.TenantID, Valid: true},
		AgentName:     params.AgentName,
		RunID:         params.RunID,
		SessionLabel:  params.SessionLabel,
		ModelUsed:     params.ModelUsed,
		ReasoningMode: params.ReasoningMode,
		StartedAt:     pgtype.Timestamptz{Time: params.StartedAt, Valid: true},
		DurationMs:    pgtype.Int4{Int32: int32(params.DurationMs), Valid: params.DurationMs > 0},
		ToolCallCount: int32(params.ToolCallCount),
		TokenInput:    int32(params.TokenInput),
		TokenOutput:   int32(params.TokenOutput),
		Outcome:       params.Outcome,
		OutcomeDetail: params.OutcomeDetail,
		CycleCount:    int32(params.CycleCount),
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.UUID(row.ID.Bytes), nil
}

func (r *Repository) CompleteAgentRun(ctx context.Context, params CompleteAgentRunParams) error {
	dbParams := leadsdb.CompleteAgentRunParams{
		ID:            pgtype.UUID{Bytes: params.ID, Valid: true},
		ToolCallCount: int32(params.ToolCallCount),
		TokenInput:    int32(params.TokenInput),
		TokenOutput:   int32(params.TokenOutput),
		Outcome:       params.Outcome,
		OutcomeDetail: params.OutcomeDetail,
	}
	if params.DurationMs != nil {
		dbParams.DurationMs = pgtype.Int4{Int32: int32(*params.DurationMs), Valid: true}
	}
	return r.queries.CompleteAgentRun(ctx, dbParams)
}

func (r *Repository) InsertAgentToolCall(ctx context.Context, params InsertAgentToolCallParams) error {
	return r.queries.InsertAgentToolCall(ctx, leadsdb.InsertAgentToolCallParams{
		AgentRunID:    pgtype.UUID{Bytes: params.AgentRunID, Valid: true},
		SequenceNum:   int32(params.SequenceNum),
		ToolName:      params.ToolName,
		ArgumentsJson: params.ArgumentsJSON,
		ResponseJson:  params.ResponseJSON,
		HasError:      params.HasError,
		ErrorMessage:  params.ErrorMessage,
		DurationMs:    int32(params.DurationMs),
	})
}

func (r *Repository) ListAgentRunsByService(ctx context.Context, serviceID, tenantID uuid.UUID, limit int) ([]AgentRun, error) {
	rows, err := r.queries.ListAgentRunsByService(ctx, leadsdb.ListAgentRunsByServiceParams{
		ServiceID: pgtype.UUID{Bytes: serviceID, Valid: true},
		TenantID:  pgtype.UUID{Bytes: tenantID, Valid: true},
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	runs := make([]AgentRun, 0, len(rows))
	for _, row := range rows {
		run := AgentRun{
			ID:            uuid.UUID(row.ID.Bytes),
			LeadID:        uuid.UUID(row.LeadID.Bytes),
			ServiceID:     uuid.UUID(row.ServiceID.Bytes),
			TenantID:      uuid.UUID(row.TenantID.Bytes),
			AgentName:     row.AgentName,
			RunID:         row.RunID,
			SessionLabel:  row.SessionLabel,
			ModelUsed:     row.ModelUsed,
			ReasoningMode: row.ReasoningMode,
			ToolCallCount: int(row.ToolCallCount),
			TokenInput:    int(row.TokenInput),
			TokenOutput:   int(row.TokenOutput),
			Outcome:       row.Outcome,
			OutcomeDetail: row.OutcomeDetail,
			CycleCount:    int(row.CycleCount),
		}
		if row.StartedAt.Valid {
			run.StartedAt = row.StartedAt.Time
		}
		if row.FinishedAt.Valid {
			t := row.FinishedAt.Time
			run.FinishedAt = &t
		}
		if row.DurationMs.Valid {
			d := int(row.DurationMs.Int32)
			run.DurationMs = &d
		}
		if row.CreatedAt.Valid {
			run.CreatedAt = row.CreatedAt.Time
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (r *Repository) GetAgentHealthStats(ctx context.Context, tenantID uuid.UUID, since time.Time) (AgentHealthStats, error) {
	row, err := r.queries.GetAgentHealthStats(ctx, leadsdb.GetAgentHealthStatsParams{
		TenantID:  pgtype.UUID{Bytes: tenantID, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return AgentHealthStats{}, err
	}
	return AgentHealthStats{
		TotalRuns:        row.TotalRuns,
		SuccessCount:     row.SuccessCount,
		FallbackCount:    row.FallbackCount,
		EscalationCount:  row.EscalationCount,
		LoopCount:        row.LoopCount,
		ErrorCount:       row.ErrorCount,
		TimeoutCount:     row.TimeoutCount,
		AvgToolCalls:     int(row.AvgToolCalls),
		AvgDurationMs:    int(row.AvgDurationMs),
		TotalTokenInput:  row.TotalTokenInput,
		TotalTokenOutput: row.TotalTokenOutput,
	}, nil
}
