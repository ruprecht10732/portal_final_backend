package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AIAnalysis represents a single AI analysis for a lead
type AIAnalysis struct {
	ID                  uuid.UUID
	LeadID              uuid.UUID
	UrgencyLevel        string // High, Medium, Low
	UrgencyReason       *string
	TalkingPoints       []string
	ObjectionHandling   []ObjectionResponse
	UpsellOpportunities []string
	Summary             string
	CreatedAt           time.Time
}

// ObjectionResponse represents an objection and its suggested response
type ObjectionResponse struct {
	Objection string `json:"objection"`
	Response  string `json:"response"`
}

// CreateAIAnalysisParams contains the parameters for creating an AI analysis
type CreateAIAnalysisParams struct {
	LeadID              uuid.UUID
	UrgencyLevel        string
	UrgencyReason       *string
	TalkingPoints       []string
	ObjectionHandling   []ObjectionResponse
	UpsellOpportunities []string
	Summary             string
}

// CreateAIAnalysis stores a new AI analysis for a lead
func (r *Repository) CreateAIAnalysis(ctx context.Context, params CreateAIAnalysisParams) (AIAnalysis, error) {
	talkingPointsJSON, _ := json.Marshal(params.TalkingPoints)
	objectionHandlingJSON, _ := json.Marshal(params.ObjectionHandling)
	upsellJSON, _ := json.Marshal(params.UpsellOpportunities)

	var analysis AIAnalysis
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lead_ai_analysis (lead_id, urgency_level, urgency_reason, talking_points, objection_handling, upsell_opportunities, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, lead_id, urgency_level, urgency_reason, talking_points, objection_handling, upsell_opportunities, summary, created_at
	`,
		params.LeadID, params.UrgencyLevel, params.UrgencyReason,
		talkingPointsJSON, objectionHandlingJSON, upsellJSON, params.Summary,
	).Scan(
		&analysis.ID, &analysis.LeadID, &analysis.UrgencyLevel, &analysis.UrgencyReason,
		&talkingPointsJSON, &objectionHandlingJSON, &upsellJSON, &analysis.Summary, &analysis.CreatedAt,
	)
	if err != nil {
		return AIAnalysis{}, err
	}

	_ = json.Unmarshal(talkingPointsJSON, &analysis.TalkingPoints)
	_ = json.Unmarshal(objectionHandlingJSON, &analysis.ObjectionHandling)
	_ = json.Unmarshal(upsellJSON, &analysis.UpsellOpportunities)

	return analysis, nil
}

// GetLatestAIAnalysis returns the most recent AI analysis for a lead
func (r *Repository) GetLatestAIAnalysis(ctx context.Context, leadID uuid.UUID) (AIAnalysis, error) {
	var analysis AIAnalysis
	var talkingPointsJSON, objectionHandlingJSON, upsellJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, urgency_level, urgency_reason, talking_points, objection_handling, upsell_opportunities, summary, created_at
		FROM lead_ai_analysis
		WHERE lead_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, leadID).Scan(
		&analysis.ID, &analysis.LeadID, &analysis.UrgencyLevel, &analysis.UrgencyReason,
		&talkingPointsJSON, &objectionHandlingJSON, &upsellJSON, &analysis.Summary, &analysis.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AIAnalysis{}, ErrNotFound
	}
	if err != nil {
		return AIAnalysis{}, err
	}

	_ = json.Unmarshal(talkingPointsJSON, &analysis.TalkingPoints)
	_ = json.Unmarshal(objectionHandlingJSON, &analysis.ObjectionHandling)
	_ = json.Unmarshal(upsellJSON, &analysis.UpsellOpportunities)

	return analysis, nil
}

// ListAIAnalyses returns all AI analyses for a lead, ordered by most recent first
func (r *Repository) ListAIAnalyses(ctx context.Context, leadID uuid.UUID) ([]AIAnalysis, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, urgency_level, urgency_reason, talking_points, objection_handling, upsell_opportunities, summary, created_at
		FROM lead_ai_analysis
		WHERE lead_id = $1
		ORDER BY created_at DESC
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var analyses []AIAnalysis
	for rows.Next() {
		var analysis AIAnalysis
		var talkingPointsJSON, objectionHandlingJSON, upsellJSON []byte

		if err := rows.Scan(
			&analysis.ID, &analysis.LeadID, &analysis.UrgencyLevel, &analysis.UrgencyReason,
			&talkingPointsJSON, &objectionHandlingJSON, &upsellJSON, &analysis.Summary, &analysis.CreatedAt,
		); err != nil {
			return nil, err
		}

		_ = json.Unmarshal(talkingPointsJSON, &analysis.TalkingPoints)
		_ = json.Unmarshal(objectionHandlingJSON, &analysis.ObjectionHandling)
		_ = json.Unmarshal(upsellJSON, &analysis.UpsellOpportunities)

		analyses = append(analyses, analysis)
	}

	return analyses, rows.Err()
}
