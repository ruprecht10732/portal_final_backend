package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	leadsdb "portal_final_backend/internal/leads/db"
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

	row, err := r.queries.CreateHumanFeedback(ctx, leadsdb.CreateHumanFeedbackParams{
		OrganizationID:  toPgUUID(params.OrganizationID),
		QuoteID:         toPgUUID(params.QuoteID),
		LeadServiceID:   toPgUUIDPtr(params.LeadServiceID),
		FieldChanged:    params.FieldChanged,
		AiValue:         aiValueJSON,
		HumanValue:      humanValueJSON,
		DeltaPercentage: toPgFloat8Ptr(delta),
	})
	if err != nil {
		return HumanFeedback{}, err
	}

	return humanFeedbackFromDB(row), nil
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

	rows, err := r.queries.ListRecentAppliedHumanFeedbackByServiceType(ctx, leadsdb.ListRecentAppliedHumanFeedbackByServiceTypeParams{
		OrganizationID: toPgUUID(organizationID),
		Name:           serviceType,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}

	items := make([]HumanFeedback, 0, limit)
	for _, row := range rows {
		items = append(items, humanFeedbackFromDB(row))
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

	row, err := r.queries.GetHumanFeedbackByID(ctx, leadsdb.GetHumanFeedbackByIDParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HumanFeedback{}, ErrNotFound
		}
		return HumanFeedback{}, err
	}

	return humanFeedbackFromDB(row), nil
}

func (r *Repository) MarkHumanFeedbackApplied(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, embeddingID *string) (HumanFeedback, error) {
	if id == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("id is required")
	}
	if organizationID == uuid.Nil {
		return HumanFeedback{}, fmt.Errorf("organization_id is required")
	}

	row, err := r.queries.MarkHumanFeedbackApplied(ctx, leadsdb.MarkHumanFeedbackAppliedParams{
		ID:                 toPgUUID(id),
		OrganizationID:     toPgUUID(organizationID),
		ContextEmbeddingID: toPgText(embeddingID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HumanFeedback{}, ErrNotFound
		}
		return HumanFeedback{}, err
	}
	return humanFeedbackFromDB(row), nil
}

func humanFeedbackFromDB(row leadsdb.RacHumanFeedback) HumanFeedback {
	item := HumanFeedback{
		ID:               uuid.UUID(row.ID.Bytes),
		OrganizationID:   uuid.UUID(row.OrganizationID.Bytes),
		QuoteID:          uuid.UUID(row.QuoteID.Bytes),
		LeadServiceID:    optionalUUID(row.LeadServiceID),
		FieldChanged:     row.FieldChanged,
		DeltaPercentage:  optionalFloat64(row.DeltaPercentage),
		ContextEmbedding: optionalString(row.ContextEmbeddingID),
		AppliedToMemory:  row.AppliedToMemory,
		CreatedAt:        row.CreatedAt.Time,
	}
	_ = json.Unmarshal(row.AiValue, &item.AIValue)
	_ = json.Unmarshal(row.HumanValue, &item.HumanValue)
	if item.AIValue == nil {
		item.AIValue = map[string]any{}
	}
	if item.HumanValue == nil {
		item.HumanValue = map[string]any{}
	}
	return item
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
