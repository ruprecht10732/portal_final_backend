package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	quotesdb "portal_final_backend/internal/quotes/db"

	"github.com/google/uuid"
)

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

func (r *Repository) CreateHumanFeedback(ctx context.Context, params CreateHumanFeedbackParams) (*HumanFeedback, error) {
	if params.OrganizationID == uuid.Nil {
		return nil, fmt.Errorf("organization_id is required")
	}
	if params.QuoteID == uuid.Nil {
		return nil, fmt.Errorf("quote_id is required")
	}
	params.FieldChanged = strings.TrimSpace(params.FieldChanged)
	if params.FieldChanged == "" {
		return nil, fmt.Errorf("field_changed is required")
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

	row, err := r.queries.CreateHumanFeedback(ctx, quotesdb.CreateHumanFeedbackParams{
		OrganizationID:  toPgUUID(params.OrganizationID),
		QuoteID:         toPgUUID(params.QuoteID),
		LeadServiceID:   toPgUUIDPtr(params.LeadServiceID),
		FieldChanged:    params.FieldChanged,
		AiValue:         aiValueJSON,
		HumanValue:      humanValueJSON,
		DeltaPercentage: toPgFloat8Ptr(delta),
	})
	if err != nil {
		return nil, err
	}

	out := &HumanFeedback{
		ID:               uuid.UUID(row.ID.Bytes),
		OrganizationID:   uuid.UUID(row.OrganizationID.Bytes),
		QuoteID:          uuid.UUID(row.QuoteID.Bytes),
		LeadServiceID:    optionalUUID(row.LeadServiceID),
		FieldChanged:     row.FieldChanged,
		DeltaPercentage:  optionalFloat(row.DeltaPercentage),
		ContextEmbedding: optionalString(row.ContextEmbeddingID),
		AppliedToMemory:  row.AppliedToMemory,
		CreatedAt:        timeFromPg(row.CreatedAt),
	}
	_ = json.Unmarshal(row.AiValue, &out.AIValue)
	_ = json.Unmarshal(row.HumanValue, &out.HumanValue)
	if out.AIValue == nil {
		out.AIValue = map[string]any{}
	}
	if out.HumanValue == nil {
		out.HumanValue = map[string]any{}
	}
	return out, nil
}

func calculateFeedbackDelta(aiValue map[string]any, humanValue map[string]any) *float64 {
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
