package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrPhotoAnalysisNotFound = errors.New("photo analysis not found")

// PhotoAnalysis represents an AI analysis of photos for a lead service.
type PhotoAnalysis struct {
	ID              uuid.UUID
	LeadID          uuid.UUID
	ServiceID       uuid.UUID
	OrganizationID  uuid.UUID
	Summary         string
	Observations    []string
	ScopeAssessment string
	CostIndicators  string
	SafetyConcerns  []string
	AdditionalInfo  []string
	ConfidenceLevel string
	PhotoCount      int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreatePhotoAnalysisParams contains parameters for creating a photo analysis.
type CreatePhotoAnalysisParams struct {
	LeadID          uuid.UUID
	ServiceID       uuid.UUID
	OrganizationID  uuid.UUID
	Summary         string
	Observations    []string
	ScopeAssessment string
	CostIndicators  string
	SafetyConcerns  []string
	AdditionalInfo  []string
	ConfidenceLevel string
	PhotoCount      int
}

// CreatePhotoAnalysis inserts a new photo analysis record.
func (r *Repository) CreatePhotoAnalysis(ctx context.Context, params CreatePhotoAnalysisParams) (PhotoAnalysis, error) {
	observationsJSON, _ := json.Marshal(params.Observations)
	safetyConcernsJSON, _ := json.Marshal(params.SafetyConcerns)
	additionalInfoJSON, _ := json.Marshal(params.AdditionalInfo)

	var pa PhotoAnalysis
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lead_photo_analyses 
			(lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count, created_at, updated_at
	`, params.LeadID, params.ServiceID, params.OrganizationID, params.Summary, observationsJSON, params.ScopeAssessment,
		params.CostIndicators, safetyConcernsJSON, additionalInfoJSON, params.ConfidenceLevel, params.PhotoCount,
	).Scan(
		&pa.ID, &pa.LeadID, &pa.ServiceID, &pa.OrganizationID, &pa.Summary, &observationsJSON, &pa.ScopeAssessment,
		&pa.CostIndicators, &safetyConcernsJSON, &additionalInfoJSON, &pa.ConfidenceLevel, &pa.PhotoCount, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if err != nil {
		return PhotoAnalysis{}, err
	}

	// Parse JSON arrays
	_ = json.Unmarshal(observationsJSON, &pa.Observations)
	_ = json.Unmarshal(safetyConcernsJSON, &pa.SafetyConcerns)
	_ = json.Unmarshal(additionalInfoJSON, &pa.AdditionalInfo)

	return pa, nil
}

// GetPhotoAnalysisByID retrieves a photo analysis by ID, scoped to organization.
func (r *Repository) GetPhotoAnalysisByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error) {
	var pa PhotoAnalysis
	var observationsJSON, safetyConcernsJSON, additionalInfoJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count, created_at, updated_at
		FROM lead_photo_analyses
		WHERE id = $1 AND org_id = $2
	`, id, organizationID).Scan(
		&pa.ID, &pa.LeadID, &pa.ServiceID, &pa.OrganizationID, &pa.Summary, &observationsJSON, &pa.ScopeAssessment,
		&pa.CostIndicators, &safetyConcernsJSON, &additionalInfoJSON, &pa.ConfidenceLevel, &pa.PhotoCount, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoAnalysis{}, ErrPhotoAnalysisNotFound
	}
	if err != nil {
		return PhotoAnalysis{}, err
	}

	_ = json.Unmarshal(observationsJSON, &pa.Observations)
	_ = json.Unmarshal(safetyConcernsJSON, &pa.SafetyConcerns)
	_ = json.Unmarshal(additionalInfoJSON, &pa.AdditionalInfo)

	return pa, nil
}

// GetLatestPhotoAnalysis retrieves the most recent photo analysis for a service.
func (r *Repository) GetLatestPhotoAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error) {
	var pa PhotoAnalysis
	var observationsJSON, safetyConcernsJSON, additionalInfoJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count, created_at, updated_at
		FROM lead_photo_analyses
		WHERE service_id = $1 AND org_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, serviceID, organizationID).Scan(
		&pa.ID, &pa.LeadID, &pa.ServiceID, &pa.OrganizationID, &pa.Summary, &observationsJSON, &pa.ScopeAssessment,
		&pa.CostIndicators, &safetyConcernsJSON, &additionalInfoJSON, &pa.ConfidenceLevel, &pa.PhotoCount, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoAnalysis{}, ErrPhotoAnalysisNotFound
	}
	if err != nil {
		return PhotoAnalysis{}, err
	}

	_ = json.Unmarshal(observationsJSON, &pa.Observations)
	_ = json.Unmarshal(safetyConcernsJSON, &pa.SafetyConcerns)
	_ = json.Unmarshal(additionalInfoJSON, &pa.AdditionalInfo)

	return pa, nil
}

// ListPhotoAnalysesByService retrieves all photo analyses for a service.
func (r *Repository) ListPhotoAnalysesByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count, created_at, updated_at
		FROM lead_photo_analyses
		WHERE service_id = $1 AND org_id = $2
		ORDER BY created_at DESC
	`, serviceID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	analyses := make([]PhotoAnalysis, 0)
	for rows.Next() {
		var pa PhotoAnalysis
		var observationsJSON, safetyConcernsJSON, additionalInfoJSON []byte

		if err := rows.Scan(
			&pa.ID, &pa.LeadID, &pa.ServiceID, &pa.OrganizationID, &pa.Summary, &observationsJSON, &pa.ScopeAssessment,
			&pa.CostIndicators, &safetyConcernsJSON, &additionalInfoJSON, &pa.ConfidenceLevel, &pa.PhotoCount, &pa.CreatedAt, &pa.UpdatedAt,
		); err != nil {
			return nil, err
		}

		_ = json.Unmarshal(observationsJSON, &pa.Observations)
		_ = json.Unmarshal(safetyConcernsJSON, &pa.SafetyConcerns)
		_ = json.Unmarshal(additionalInfoJSON, &pa.AdditionalInfo)

		analyses = append(analyses, pa)
	}
	return analyses, rows.Err()
}

// ListPhotoAnalysesByLead retrieves all photo analyses for a lead.
func (r *Repository) ListPhotoAnalysesByLead(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators, safety_concerns, additional_info, confidence_level, photo_count, created_at, updated_at
		FROM lead_photo_analyses
		WHERE lead_id = $1 AND org_id = $2
		ORDER BY created_at DESC
	`, leadID, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	analyses := make([]PhotoAnalysis, 0)
	for rows.Next() {
		var pa PhotoAnalysis
		var observationsJSON, safetyConcernsJSON, additionalInfoJSON []byte

		if err := rows.Scan(
			&pa.ID, &pa.LeadID, &pa.ServiceID, &pa.OrganizationID, &pa.Summary, &observationsJSON, &pa.ScopeAssessment,
			&pa.CostIndicators, &safetyConcernsJSON, &additionalInfoJSON, &pa.ConfidenceLevel, &pa.PhotoCount, &pa.CreatedAt, &pa.UpdatedAt,
		); err != nil {
			return nil, err
		}

		_ = json.Unmarshal(observationsJSON, &pa.Observations)
		_ = json.Unmarshal(safetyConcernsJSON, &pa.SafetyConcerns)
		_ = json.Unmarshal(additionalInfoJSON, &pa.AdditionalInfo)

		analyses = append(analyses, pa)
	}
	return analyses, rows.Err()
}
