package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	leadsdb "portal_final_backend/internal/leads/db"
)

const aiAnalysisSelectColumns = `id, lead_id, organization_id, lead_service_id, urgency_level, urgency_reason,
		lead_quality, recommended_action, missing_information,
		preferred_contact_channel, suggested_contact_message, summary,
		composite_confidence, confidence_breakdown, risk_flags, created_at`

// AIAnalysis represents a single AI analysis for a lead service.
type AIAnalysis struct {
	ID                      uuid.UUID
	LeadID                  uuid.UUID
	OrganizationID          uuid.UUID
	LeadServiceID           uuid.UUID
	UrgencyLevel            string
	UrgencyReason           *string
	LeadQuality             string
	RecommendedAction       string
	MissingInformation      []string
	CompositeConfidence     *float64
	ConfidenceBreakdown     map[string]float64
	RiskFlags               []string
	PreferredContactChannel string
	SuggestedContactMessage string
	Summary                 string
	CreatedAt               time.Time
}

// CreateAIAnalysisParams contains the parameters for creating an AI analysis.
type CreateAIAnalysisParams struct {
	LeadID                  uuid.UUID
	OrganizationID          uuid.UUID
	LeadServiceID           uuid.UUID
	UrgencyLevel            string
	UrgencyReason           *string
	LeadQuality             string
	RecommendedAction       string
	MissingInformation      []string
	CompositeConfidence     *float64
	ConfidenceBreakdown     map[string]float64
	RiskFlags               []string
	PreferredContactChannel string
	SuggestedContactMessage string
	Summary                 string
}

// CreateAIAnalysis stores a new AI analysis for a lead service.
func (r *Repository) CreateAIAnalysis(ctx context.Context, params CreateAIAnalysisParams) (AIAnalysis, error) {
	missingInfoJSON := marshalJSONArray(params.MissingInformation)
	breakdownJSON := marshalJSONMap(params.ConfidenceBreakdown)
	riskFlagsJSON := marshalJSONArray(params.RiskFlags)

	row, err := r.queries.CreateAIAnalysis(ctx, leadsdb.CreateAIAnalysisParams{
		LeadID:                  toPgUUID(params.LeadID),
		OrganizationID:          toPgUUID(params.OrganizationID),
		LeadServiceID:           toPgUUID(params.LeadServiceID),
		UrgencyLevel:            params.UrgencyLevel,
		UrgencyReason:           toPgText(params.UrgencyReason),
		LeadQuality:             params.LeadQuality,
		RecommendedAction:       params.RecommendedAction,
		MissingInformation:      missingInfoJSON,
		PreferredContactChannel: params.PreferredContactChannel,
		SuggestedContactMessage: params.SuggestedContactMessage,
		Summary:                 params.Summary,
		CompositeConfidence:     toPgFloat8Ptr(params.CompositeConfidence),
		ConfidenceBreakdown:     breakdownJSON,
		RiskFlags:               riskFlagsJSON,
	})
	if err != nil {
		return AIAnalysis{}, err
	}
	return aiAnalysisSnapshot{
		id:                      row.ID,
		leadID:                  row.LeadID,
		organizationID:          row.OrganizationID,
		leadServiceID:           row.LeadServiceID,
		urgencyLevel:            row.UrgencyLevel,
		urgencyReason:           row.UrgencyReason,
		leadQuality:             row.LeadQuality,
		recommendedAction:       row.RecommendedAction,
		missingInformation:      row.MissingInformation,
		compositeConfidence:     row.CompositeConfidence,
		confidenceBreakdown:     row.ConfidenceBreakdown,
		riskFlags:               row.RiskFlags,
		preferredContactChannel: row.PreferredContactChannel,
		suggestedContactMessage: row.SuggestedContactMessage,
		summary:                 row.Summary,
		createdAt:               row.CreatedAt,
	}.toModel(), nil
}

// GetLatestAIAnalysis returns the most recent AI analysis for a service.
func (r *Repository) GetLatestAIAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (AIAnalysis, error) {
	row, err := r.queries.GetLatestAIAnalysis(ctx, leadsdb.GetLatestAIAnalysisParams{LeadServiceID: toPgUUID(serviceID), OrganizationID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return AIAnalysis{}, ErrNotFound
	}
	if err != nil {
		return AIAnalysis{}, err
	}
	return aiAnalysisSnapshot{
		id:                      row.ID,
		leadID:                  row.LeadID,
		organizationID:          row.OrganizationID,
		leadServiceID:           row.LeadServiceID,
		urgencyLevel:            row.UrgencyLevel,
		urgencyReason:           row.UrgencyReason,
		leadQuality:             row.LeadQuality,
		recommendedAction:       row.RecommendedAction,
		missingInformation:      row.MissingInformation,
		compositeConfidence:     row.CompositeConfidence,
		confidenceBreakdown:     row.ConfidenceBreakdown,
		riskFlags:               row.RiskFlags,
		preferredContactChannel: row.PreferredContactChannel,
		suggestedContactMessage: row.SuggestedContactMessage,
		summary:                 row.Summary,
		createdAt:               row.CreatedAt,
	}.toModel(), nil
}

// ListAIAnalyses returns all AI analyses for a service, ordered by most recent first.
func (r *Repository) ListAIAnalyses(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]AIAnalysis, error) {
	rows, err := r.queries.ListAIAnalyses(ctx, leadsdb.ListAIAnalysesParams{LeadServiceID: toPgUUID(serviceID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	analyses := make([]AIAnalysis, 0, len(rows))
	for _, row := range rows {
		analyses = append(analyses, aiAnalysisSnapshot{
			id:                      row.ID,
			leadID:                  row.LeadID,
			organizationID:          row.OrganizationID,
			leadServiceID:           row.LeadServiceID,
			urgencyLevel:            row.UrgencyLevel,
			urgencyReason:           row.UrgencyReason,
			leadQuality:             row.LeadQuality,
			recommendedAction:       row.RecommendedAction,
			missingInformation:      row.MissingInformation,
			compositeConfidence:     row.CompositeConfidence,
			confidenceBreakdown:     row.ConfidenceBreakdown,
			riskFlags:               row.RiskFlags,
			preferredContactChannel: row.PreferredContactChannel,
			suggestedContactMessage: row.SuggestedContactMessage,
			summary:                 row.Summary,
			createdAt:               row.CreatedAt,
		}.toModel())
	}
	return analyses, nil
}

type aiAnalysisSnapshot struct {
	id                      pgtype.UUID
	leadID                  pgtype.UUID
	organizationID          pgtype.UUID
	leadServiceID           pgtype.UUID
	urgencyLevel            string
	urgencyReason           pgtype.Text
	leadQuality             string
	recommendedAction       string
	missingInformation      []byte
	compositeConfidence     pgtype.Float8
	confidenceBreakdown     []byte
	riskFlags               []byte
	preferredContactChannel string
	suggestedContactMessage string
	summary                 string
	createdAt               pgtype.Timestamptz
}

func (snapshot aiAnalysisSnapshot) toModel() AIAnalysis {
	analysis := AIAnalysis{
		ID:                      snapshot.id.Bytes,
		LeadID:                  snapshot.leadID.Bytes,
		OrganizationID:          snapshot.organizationID.Bytes,
		LeadServiceID:           snapshot.leadServiceID.Bytes,
		UrgencyLevel:            snapshot.urgencyLevel,
		UrgencyReason:           optionalString(snapshot.urgencyReason),
		LeadQuality:             snapshot.leadQuality,
		RecommendedAction:       snapshot.recommendedAction,
		CompositeConfidence:     optionalFloat64(snapshot.compositeConfidence),
		PreferredContactChannel: snapshot.preferredContactChannel,
		SuggestedContactMessage: snapshot.suggestedContactMessage,
		Summary:                 snapshot.summary,
		CreatedAt:               snapshot.createdAt.Time,
	}
	_ = json.Unmarshal(snapshot.missingInformation, &analysis.MissingInformation)
	_ = json.Unmarshal(snapshot.confidenceBreakdown, &analysis.ConfidenceBreakdown)
	_ = json.Unmarshal(snapshot.riskFlags, &analysis.RiskFlags)
	if analysis.ConfidenceBreakdown == nil {
		analysis.ConfidenceBreakdown = map[string]float64{}
	}
	if analysis.MissingInformation == nil {
		analysis.MissingInformation = []string{}
	}
	if analysis.RiskFlags == nil {
		analysis.RiskFlags = []string{}
	}
	return analysis
}

func marshalJSONArray(values []string) []byte {
	if values == nil {
		values = []string{}
	}
	data, _ := json.Marshal(values)
	return data
}

func marshalJSONMap(values map[string]float64) []byte {
	if values == nil {
		values = map[string]float64{}
	}
	data, _ := json.Marshal(values)
	return data
}
