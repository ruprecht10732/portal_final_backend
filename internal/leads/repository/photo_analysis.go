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

var ErrPhotoAnalysisNotFound = errors.New("photo analysis not found")

// Measurement represents a single measurement extracted from a photo.
type Measurement struct {
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Type        string  `json:"type"`
	Confidence  string  `json:"confidence"`
	PhotoRef    string  `json:"photoRef,omitempty"`
}

// PhotoAnalysis represents a forensic AI analysis of photos for a lead service.
type PhotoAnalysis struct {
	ID                     uuid.UUID     `json:"id"`
	LeadID                 uuid.UUID     `json:"leadId"`
	ServiceID              uuid.UUID     `json:"serviceId"`
	OrganizationID         uuid.UUID     `json:"-"`
	Summary                string        `json:"summary"`
	Observations           []string      `json:"observations"`
	ScopeAssessment        string        `json:"scopeAssessment"`
	CostIndicators         string        `json:"costIndicators"`
	SafetyConcerns         []string      `json:"safetyConcerns"`
	AdditionalInfo         []string      `json:"additionalInfo"`
	ConfidenceLevel        string        `json:"confidenceLevel"`
	PhotoCount             int           `json:"photoCount"`
	Measurements           []Measurement `json:"measurements"`
	NeedsOnsiteMeasurement []string      `json:"needsOnsiteMeasurement"`
	Discrepancies          []string      `json:"discrepancies"`
	ExtractedText          []string      `json:"extractedText"`
	SuggestedSearchTerms   []string      `json:"suggestedSearchTerms"`
	CreatedAt              time.Time     `json:"createdAt"`
	UpdatedAt              time.Time     `json:"updatedAt"`
}

// CreatePhotoAnalysisParams contains parameters for creating a photo analysis.
type CreatePhotoAnalysisParams struct {
	LeadID                 uuid.UUID
	ServiceID              uuid.UUID
	OrganizationID         uuid.UUID
	Summary                string
	Observations           []string
	ScopeAssessment        string
	CostIndicators         string
	SafetyConcerns         []string
	AdditionalInfo         []string
	ConfidenceLevel        string
	PhotoCount             int
	Measurements           []Measurement
	NeedsOnsiteMeasurement []string
	Discrepancies          []string
	ExtractedText          []string
	SuggestedSearchTerms   []string
}

// CreatePhotoAnalysis inserts a new photo analysis record.
func (r *Repository) CreatePhotoAnalysis(ctx context.Context, params CreatePhotoAnalysisParams) (PhotoAnalysis, error) {
	row, err := r.queries.CreatePhotoAnalysis(ctx, leadsdb.CreatePhotoAnalysisParams{
		LeadID:                 toPgUUID(params.LeadID),
		ServiceID:              toPgUUID(params.ServiceID),
		OrgID:                  toPgUUID(params.OrganizationID),
		Summary:                params.Summary,
		Observations:           marshalJSONStrings(params.Observations),
		ScopeAssessment:        params.ScopeAssessment,
		CostIndicators:         toPgTextValue(params.CostIndicators),
		SafetyConcerns:         marshalJSONStrings(params.SafetyConcerns),
		AdditionalInfo:         marshalJSONStrings(params.AdditionalInfo),
		ConfidenceLevel:        params.ConfidenceLevel,
		PhotoCount:             int32(params.PhotoCount),
		Measurements:           marshalJSONMeasurements(params.Measurements),
		NeedsOnsiteMeasurement: marshalJSONStrings(params.NeedsOnsiteMeasurement),
		Discrepancies:          marshalJSONStrings(params.Discrepancies),
		ExtractedText:          marshalJSONStrings(params.ExtractedText),
		SuggestedSearchTerms:   marshalJSONStrings(params.SuggestedSearchTerms),
	})
	if err != nil {
		return PhotoAnalysis{}, err
	}
	return photoAnalysisFromRow(row.ID, row.LeadID, row.ServiceID, row.OrgID, row.Summary, row.Observations, row.ScopeAssessment, row.CostIndicators, row.SafetyConcerns, row.AdditionalInfo, row.ConfidenceLevel, row.PhotoCount, row.Measurements, row.NeedsOnsiteMeasurement, row.Discrepancies, row.ExtractedText, row.SuggestedSearchTerms, row.CreatedAt, row.UpdatedAt), nil
}

// GetPhotoAnalysisByID retrieves a photo analysis by ID, scoped to organization.
func (r *Repository) GetPhotoAnalysisByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error) {
	row, err := r.queries.GetPhotoAnalysisByID(ctx, leadsdb.GetPhotoAnalysisByIDParams{ID: toPgUUID(id), OrgID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoAnalysis{}, ErrPhotoAnalysisNotFound
	}
	if err != nil {
		return PhotoAnalysis{}, err
	}
	return photoAnalysisFromRow(row.ID, row.LeadID, row.ServiceID, row.OrgID, row.Summary, row.Observations, row.ScopeAssessment, row.CostIndicators, row.SafetyConcerns, row.AdditionalInfo, row.ConfidenceLevel, row.PhotoCount, row.Measurements, row.NeedsOnsiteMeasurement, row.Discrepancies, row.ExtractedText, row.SuggestedSearchTerms, row.CreatedAt, row.UpdatedAt), nil
}

// GetLatestPhotoAnalysis retrieves the most recent photo analysis for a service.
func (r *Repository) GetLatestPhotoAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error) {
	row, err := r.queries.GetLatestPhotoAnalysis(ctx, leadsdb.GetLatestPhotoAnalysisParams{ServiceID: toPgUUID(serviceID), OrgID: toPgUUID(organizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return PhotoAnalysis{}, ErrPhotoAnalysisNotFound
	}
	if err != nil {
		return PhotoAnalysis{}, err
	}
	return photoAnalysisFromRow(row.ID, row.LeadID, row.ServiceID, row.OrgID, row.Summary, row.Observations, row.ScopeAssessment, row.CostIndicators, row.SafetyConcerns, row.AdditionalInfo, row.ConfidenceLevel, row.PhotoCount, row.Measurements, row.NeedsOnsiteMeasurement, row.Discrepancies, row.ExtractedText, row.SuggestedSearchTerms, row.CreatedAt, row.UpdatedAt), nil
}

// ListPhotoAnalysesByService retrieves all photo analyses for a service.
func (r *Repository) ListPhotoAnalysesByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error) {
	rows, err := r.queries.ListPhotoAnalysesByService(ctx, leadsdb.ListPhotoAnalysesByServiceParams{ServiceID: toPgUUID(serviceID), OrgID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	analyses := make([]PhotoAnalysis, 0, len(rows))
	for _, row := range rows {
		analyses = append(analyses, photoAnalysisFromRow(row.ID, row.LeadID, row.ServiceID, row.OrgID, row.Summary, row.Observations, row.ScopeAssessment, row.CostIndicators, row.SafetyConcerns, row.AdditionalInfo, row.ConfidenceLevel, row.PhotoCount, row.Measurements, row.NeedsOnsiteMeasurement, row.Discrepancies, row.ExtractedText, row.SuggestedSearchTerms, row.CreatedAt, row.UpdatedAt))
	}
	return analyses, nil
}

// ListPhotoAnalysesByLead retrieves all photo analyses for a lead.
func (r *Repository) ListPhotoAnalysesByLead(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error) {
	rows, err := r.queries.ListPhotoAnalysesByLead(ctx, leadsdb.ListPhotoAnalysesByLeadParams{LeadID: toPgUUID(leadID), OrgID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	analyses := make([]PhotoAnalysis, 0, len(rows))
	for _, row := range rows {
		analyses = append(analyses, photoAnalysisFromRow(row.ID, row.LeadID, row.ServiceID, row.OrgID, row.Summary, row.Observations, row.ScopeAssessment, row.CostIndicators, row.SafetyConcerns, row.AdditionalInfo, row.ConfidenceLevel, row.PhotoCount, row.Measurements, row.NeedsOnsiteMeasurement, row.Discrepancies, row.ExtractedText, row.SuggestedSearchTerms, row.CreatedAt, row.UpdatedAt))
	}
	return analyses, nil
}

func photoAnalysisFromRow(id, leadID, serviceID, organizationID pgtype.UUID, summary string, observations []byte, scopeAssessment string, costIndicators pgtype.Text, safetyConcerns, additionalInfo []byte, confidenceLevel string, photoCount int32, measurements, needsOnsiteMeasurement, discrepancies, extractedText, suggestedSearchTerms []byte, createdAt, updatedAt pgtype.Timestamptz) PhotoAnalysis {
	analysis := PhotoAnalysis{
		ID:              id.Bytes,
		LeadID:          leadID.Bytes,
		ServiceID:       serviceID.Bytes,
		OrganizationID:  organizationID.Bytes,
		Summary:         summary,
		ScopeAssessment: scopeAssessment,
		ConfidenceLevel: confidenceLevel,
		PhotoCount:      int(photoCount),
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}
	if value := optionalString(costIndicators); value != nil {
		analysis.CostIndicators = *value
	}
	_ = json.Unmarshal(observations, &analysis.Observations)
	_ = json.Unmarshal(safetyConcerns, &analysis.SafetyConcerns)
	_ = json.Unmarshal(additionalInfo, &analysis.AdditionalInfo)
	_ = json.Unmarshal(measurements, &analysis.Measurements)
	_ = json.Unmarshal(needsOnsiteMeasurement, &analysis.NeedsOnsiteMeasurement)
	_ = json.Unmarshal(discrepancies, &analysis.Discrepancies)
	_ = json.Unmarshal(extractedText, &analysis.ExtractedText)
	_ = json.Unmarshal(suggestedSearchTerms, &analysis.SuggestedSearchTerms)
	return analysis
}

func marshalJSONStrings(values []string) []byte {
	if values == nil {
		values = []string{}
	}
	data, _ := json.Marshal(values)
	return data
}

func marshalJSONMeasurements(values []Measurement) []byte {
	if values == nil {
		values = []Measurement{}
	}
	data, _ := json.Marshal(values)
	return data
}
