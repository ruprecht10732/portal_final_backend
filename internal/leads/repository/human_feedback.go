package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const organizationIDRequiredMessage = "organization_id is required"

// HumanFeedback captures a human correction against an AI-generated quote field.
type HumanFeedback struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	QuoteID          uuid.UUID
	LeadServiceID    *uuid.UUID
	FieldChanged     string
	AIValue          map[string]any
	HumanValue       map[string]any
	DeltaPercentage  *float64
	ContextEmbedding *string
	AppliedToMemory  bool
	CreatedAt        time.Time
}

type CreateHumanFeedbackParams struct {
	OrganizationID uuid.UUID
	QuoteID        uuid.UUID
	LeadServiceID  *uuid.UUID
	FieldChanged   string
	AIValue        map[string]any
	HumanValue     map[string]any
}

func (r *Repository) CreateHumanFeedback(ctx context.Context, params CreateHumanFeedbackParams) (HumanFeedback, error) {
	if params.OrganizationID == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf(organizationIDRequiredMessage)
	}
	if params.QuoteID == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("quote_id is required")
	}
	params.FieldChanged = strings.TrimSpace(params.FieldChanged)
	if params.FieldChanged == "" {
		return HumanFeedback{}, fmt.Errorf("field_changed is required")
	}
	if params.AIValue == nil {
		params.AIValue = map[string]any{}
	}
	if params.HumanValue == nil {
		params.HumanValue = map[string]any{}
	}

	aiValueJSON, _ := json.Marshal(params.AIValue)
	humanValueJSON, _ := json.Marshal(params.HumanValue)
	delta := calculateFeedbackDelta(params.AIValue, params.HumanValue)

	var out HumanFeedback
	var aiRaw []byte
	var humanRaw []byte
	var deltaNullable sql.NullFloat64
	var embeddingNullable sql.NullString
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO RAC_human_feedback (
			organization_id, quote_id, lead_service_id,
			field_changed, ai_value, human_value, delta_percentage
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, organization_id, quote_id, lead_service_id,
		          field_changed, ai_value, human_value, delta_percentage,
		          context_embedding_id, applied_to_memory, created_at
	`,
		params.OrganizationID,
		params.QuoteID,
		params.LeadServiceID,
		params.FieldChanged,
		aiValueJSON,
		humanValueJSON,
		delta,
	).Scan(
		&out.ID,
		&out.OrganizationID,
		&out.QuoteID,
		&out.LeadServiceID,
		&out.FieldChanged,
		&aiRaw,
		&humanRaw,
		&deltaNullable,
		&embeddingNullable,
		&out.AppliedToMemory,
		&out.CreatedAt,
	); err != nil {
		return HumanFeedback{}, err
	}

	_ = json.Unmarshal(aiRaw, &out.AIValue)
	_ = json.Unmarshal(humanRaw, &out.HumanValue)
	if deltaNullable.Valid {
		d := deltaNullable.Float64
		out.DeltaPercentage = &d
	}
	if embeddingNullable.Valid {
		e := embeddingNullable.String
		out.ContextEmbedding = &e
	}
	return out, nil
}

func (r *Repository) ListRecentAppliedHumanFeedbackByServiceType(ctx context.Context, organizationID uuid.UUID, serviceType string, limit int) ([]HumanFeedback, error) {
	if organizationID == uuid.Nil {
		return nil, fmt.Errorf(organizationIDRequiredMessage)
	}
	serviceType = strings.TrimSpace(serviceType)
	if serviceType == "" {
		return nil, fmt.Errorf("service_type is required")
	}
	if limit <= 0 {
		limit = 6
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT hf.id, hf.organization_id, hf.quote_id, hf.lead_service_id,
		       hf.field_changed, hf.ai_value, hf.human_value, hf.delta_percentage,
		       hf.context_embedding_id, hf.applied_to_memory, hf.created_at
		FROM RAC_human_feedback hf
		JOIN RAC_lead_services ls
		  ON ls.id = hf.lead_service_id
		 AND ls.organization_id = hf.organization_id
		JOIN RAC_service_types st
		  ON st.id = ls.service_type_id
		 AND st.organization_id = ls.organization_id
		WHERE hf.organization_id = $1
		  AND hf.applied_to_memory = true
		  AND st.name = $2
		ORDER BY hf.created_at DESC
		LIMIT $3
	`, organizationID, serviceType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]HumanFeedback, 0, limit)
	for rows.Next() {
		var item HumanFeedback
		var aiRaw []byte
		var humanRaw []byte
		var deltaNullable sql.NullFloat64
		var embeddingNullable sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.OrganizationID,
			&item.QuoteID,
			&item.LeadServiceID,
			&item.FieldChanged,
			&aiRaw,
			&humanRaw,
			&deltaNullable,
			&embeddingNullable,
			&item.AppliedToMemory,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(aiRaw, &item.AIValue)
		_ = json.Unmarshal(humanRaw, &item.HumanValue)
		if deltaNullable.Valid {
			d := deltaNullable.Float64
			item.DeltaPercentage = &d
		}
		if embeddingNullable.Valid {
			e := embeddingNullable.String
			item.ContextEmbedding = &e
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}

func (r *Repository) GetHumanFeedbackByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (HumanFeedback, error) {
	if id == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("id is required")
	}
	if organizationID == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf(organizationIDRequiredMessage)
	}

	var out HumanFeedback
	var aiRaw []byte
	var humanRaw []byte
	var deltaNullable sql.NullFloat64
	var embeddingNullable sql.NullString
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, quote_id, lead_service_id,
		       field_changed, ai_value, human_value, delta_percentage,
		       context_embedding_id, applied_to_memory, created_at
		FROM RAC_human_feedback
		WHERE id = $1 AND organization_id = $2
	`, id, organizationID).Scan(
		&out.ID,
		&out.OrganizationID,
		&out.QuoteID,
		&out.LeadServiceID,
		&out.FieldChanged,
		&aiRaw,
		&humanRaw,
		&deltaNullable,
		&embeddingNullable,
		&out.AppliedToMemory,
		&out.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HumanFeedback{}, ErrNotFound
		}
		return HumanFeedback{}, err
	}

	_ = json.Unmarshal(aiRaw, &out.AIValue)
	_ = json.Unmarshal(humanRaw, &out.HumanValue)
	if deltaNullable.Valid {
		d := deltaNullable.Float64
		out.DeltaPercentage = &d
	}
	if embeddingNullable.Valid {
		e := embeddingNullable.String
		out.ContextEmbedding = &e
	}

	return out, nil
}

func (r *Repository) MarkHumanFeedbackApplied(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, embeddingID *string) (HumanFeedback, error) {
	if id == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("id is required")
	}
	if organizationID == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("organization_id is required")
	}

	var out HumanFeedback
	var aiRaw []byte
	var humanRaw []byte
	var deltaNullable sql.NullFloat64
	var embeddingNullable sql.NullString
	if err := r.pool.QueryRow(ctx, `
		UPDATE RAC_human_feedback
		SET applied_to_memory = true,
		    context_embedding_id = COALESCE($3, context_embedding_id)
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, quote_id, lead_service_id,
		          field_changed, ai_value, human_value, delta_percentage,
		          context_embedding_id, applied_to_memory, created_at
	`, id, organizationID, embeddingID).Scan(
		&out.ID,
		&out.OrganizationID,
		&out.QuoteID,
		&out.LeadServiceID,
		&out.FieldChanged,
		&aiRaw,
		&humanRaw,
		&deltaNullable,
		&embeddingNullable,
		&out.AppliedToMemory,
		&out.CreatedAt,
	); err != nil {
		return HumanFeedback{}, err
	}

	_ = json.Unmarshal(aiRaw, &out.AIValue)
	_ = json.Unmarshal(humanRaw, &out.HumanValue)
	if deltaNullable.Valid {
		d := deltaNullable.Float64
		out.DeltaPercentage = &d
	}
	if embeddingNullable.Valid {
		e := embeddingNullable.String
		out.ContextEmbedding = &e
	}
	return out, nil
}

func calculateFeedbackDelta(aiValue, humanValue map[string]any) *float64 {
	aiNumeric, okAI := extractNumericValue(aiValue)
	humanNumeric, okHuman := extractNumericValue(humanValue)
	if !okAI || !okHuman || aiNumeric == 0 {
		return nil
	}
	value := ((humanNumeric - aiNumeric) / aiNumeric) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func extractNumericValue(value map[string]any) (float64, bool) {
	if len(value) == 0 {
		return 0, false
	}
	if raw, ok := value["value"]; ok {
		n, ok := asFloat64(raw)
		if ok {
			return n, true
		}
	}
	for _, raw := range value {
		n, ok := asFloat64(raw)
		if ok {
			return n, true
		}
	}
	return 0, false
}

func asFloat64(v any) (float64, bool) {
	switch value := v.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}
