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
	IsRelevant             *bool         `json:"isRelevant,omitempty"`
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
	IsRelevant             *bool
}

// CreatePhotoAnalysis inserts a new photo analysis record.
func (r *Repository) CreatePhotoAnalysis(ctx context.Context, params CreatePhotoAnalysisParams) (PhotoAnalysis, error) {
	row, err := r.queries.CreatePhotoAnalysis(ctx, leadsdb.CreatePhotoAnalysisParams{
		LeadID:                 toPgUUID(params.LeadID),
		ServiceID:              toPgUUID(params.ServiceID),
		OrgID:                  toPgUUID(params.OrganizationID),
		Summary:                params.Summary,
		Observations:           marshalJSONSlice(params.Observations),
		ScopeAssessment:        params.ScopeAssessment,
		CostIndicators:         toPgTextValue(params.CostIndicators),
		SafetyConcerns:         marshalJSONSlice(params.SafetyConcerns),
		AdditionalInfo:         marshalJSONSlice(params.AdditionalInfo),
		ConfidenceLevel:        params.ConfidenceLevel,
		PhotoCount:             int32(params.PhotoCount),
		Measurements:           marshalJSONSlice(params.Measurements),
		NeedsOnsiteMeasurement: marshalJSONSlice(params.NeedsOnsiteMeasurement),
		Discrepancies:          marshalJSONSlice(params.Discrepancies),
		ExtractedText:          marshalJSONSlice(params.ExtractedText),
		SuggestedSearchTerms:   marshalJSONSlice(params.SuggestedSearchTerms),
		IsRelevant:             toPgBoolPtr(params.IsRelevant),
	})
	if err != nil {
		return PhotoAnalysis{}, err
	}
	return photoAnalysisSnapshot{
		id:                     row.ID,
		leadID:                 row.LeadID,
		serviceID:              row.ServiceID,
		organizationID:         row.OrgID,
		summary:                row.Summary,
		observations:           row.Observations,
		scopeAssessment:        row.ScopeAssessment,
		costIndicators:         row.CostIndicators,
		safetyConcerns:         row.SafetyConcerns,
		additionalInfo:         row.AdditionalInfo,
		confidenceLevel:        row.ConfidenceLevel,
		photoCount:             row.PhotoCount,
		measurements:           row.Measurements,
		needsOnsiteMeasurement: row.NeedsOnsiteMeasurement,
		discrepancies:          row.Discrepancies,
		extractedText:          row.ExtractedText,
		suggestedSearchTerms:   row.SuggestedSearchTerms,
		isRelevant:             row.IsRelevant,
		createdAt:              row.CreatedAt,
		updatedAt:              row.UpdatedAt,
	}.toModel(), nil
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
	return photoAnalysisSnapshot{
		id:                     row.ID,
		leadID:                 row.LeadID,
		serviceID:              row.ServiceID,
		organizationID:         row.OrgID,
		summary:                row.Summary,
		observations:           row.Observations,
		scopeAssessment:        row.ScopeAssessment,
		costIndicators:         row.CostIndicators,
		safetyConcerns:         row.SafetyConcerns,
		additionalInfo:         row.AdditionalInfo,
		confidenceLevel:        row.ConfidenceLevel,
		photoCount:             row.PhotoCount,
		measurements:           row.Measurements,
		needsOnsiteMeasurement: row.NeedsOnsiteMeasurement,
		discrepancies:          row.Discrepancies,
		extractedText:          row.ExtractedText,
		suggestedSearchTerms:   row.SuggestedSearchTerms,
		isRelevant:             row.IsRelevant,
		createdAt:              row.CreatedAt,
		updatedAt:              row.UpdatedAt,
	}.toModel(), nil
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
	return photoAnalysisSnapshot{
		id:                     row.ID,
		leadID:                 row.LeadID,
		serviceID:              row.ServiceID,
		organizationID:         row.OrgID,
		summary:                row.Summary,
		observations:           row.Observations,
		scopeAssessment:        row.ScopeAssessment,
		costIndicators:         row.CostIndicators,
		safetyConcerns:         row.SafetyConcerns,
		additionalInfo:         row.AdditionalInfo,
		confidenceLevel:        row.ConfidenceLevel,
		photoCount:             row.PhotoCount,
		measurements:           row.Measurements,
		needsOnsiteMeasurement: row.NeedsOnsiteMeasurement,
		discrepancies:          row.Discrepancies,
		extractedText:          row.ExtractedText,
		suggestedSearchTerms:   row.SuggestedSearchTerms,
		isRelevant:             row.IsRelevant,
		createdAt:              row.CreatedAt,
		updatedAt:              row.UpdatedAt,
	}.toModel(), nil
}

// ListPhotoAnalysesByService retrieves all photo analyses for a service.
func (r *Repository) ListPhotoAnalysesByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error) {
	rows, err := r.queries.ListPhotoAnalysesByService(ctx, leadsdb.ListPhotoAnalysesByServiceParams{ServiceID: toPgUUID(serviceID), OrgID: toPgUUID(organizationID)})
	if err != nil {
		return nil, err
	}

	analyses := make([]PhotoAnalysis, 0, len(rows))
	for _, row := range rows {
		analyses = append(analyses, photoAnalysisSnapshot{
			id:                     row.ID,
			leadID:                 row.LeadID,
			serviceID:              row.ServiceID,
			organizationID:         row.OrgID,
			summary:                row.Summary,
			observations:           row.Observations,
			scopeAssessment:        row.ScopeAssessment,
			costIndicators:         row.CostIndicators,
			safetyConcerns:         row.SafetyConcerns,
			additionalInfo:         row.AdditionalInfo,
			confidenceLevel:        row.ConfidenceLevel,
			photoCount:             row.PhotoCount,
			measurements:           row.Measurements,
			needsOnsiteMeasurement: row.NeedsOnsiteMeasurement,
			discrepancies:          row.Discrepancies,
			extractedText:          row.ExtractedText,
			suggestedSearchTerms:   row.SuggestedSearchTerms,
			isRelevant:             row.IsRelevant,
			createdAt:              row.CreatedAt,
			updatedAt:              row.UpdatedAt,
		}.toModel())
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
		analyses = append(analyses, photoAnalysisSnapshot{
			id:                     row.ID,
			leadID:                 row.LeadID,
			serviceID:              row.ServiceID,
			organizationID:         row.OrgID,
			summary:                row.Summary,
			observations:           row.Observations,
			scopeAssessment:        row.ScopeAssessment,
			costIndicators:         row.CostIndicators,
			safetyConcerns:         row.SafetyConcerns,
			additionalInfo:         row.AdditionalInfo,
			confidenceLevel:        row.ConfidenceLevel,
			photoCount:             row.PhotoCount,
			measurements:           row.Measurements,
			needsOnsiteMeasurement: row.NeedsOnsiteMeasurement,
			discrepancies:          row.Discrepancies,
			extractedText:          row.ExtractedText,
			suggestedSearchTerms:   row.SuggestedSearchTerms,
			isRelevant:             row.IsRelevant,
			createdAt:              row.CreatedAt,
			updatedAt:              row.UpdatedAt,
		}.toModel())
	}
	return analyses, nil
}

type photoAnalysisSnapshot struct {
	id                     pgtype.UUID
	leadID                 pgtype.UUID
	serviceID              pgtype.UUID
	organizationID         pgtype.UUID
	summary                string
	observations           []byte
	scopeAssessment        string
	costIndicators         pgtype.Text
	safetyConcerns         []byte
	additionalInfo         []byte
	confidenceLevel        string
	photoCount             int32
	measurements           []byte
	needsOnsiteMeasurement []byte
	discrepancies          []byte
	extractedText          []byte
	suggestedSearchTerms   []byte
	isRelevant             pgtype.Bool
	createdAt              pgtype.Timestamptz
	updatedAt              pgtype.Timestamptz
}

func (snapshot photoAnalysisSnapshot) toModel() PhotoAnalysis {
	analysis := PhotoAnalysis{
		ID:              snapshot.id.Bytes,
		LeadID:          snapshot.leadID.Bytes,
		ServiceID:       snapshot.serviceID.Bytes,
		OrganizationID:  snapshot.organizationID.Bytes,
		Summary:         snapshot.summary,
		ScopeAssessment: snapshot.scopeAssessment,
		ConfidenceLevel: snapshot.confidenceLevel,
		PhotoCount:      int(snapshot.photoCount),
		CreatedAt:       snapshot.createdAt.Time,
		UpdatedAt:       snapshot.updatedAt.Time,
	}
	if value := optionalString(snapshot.costIndicators); value != nil {
		analysis.CostIndicators = *value
	}
	if snapshot.isRelevant.Valid {
		analysis.IsRelevant = &snapshot.isRelevant.Bool
	}
	_ = json.Unmarshal(snapshot.observations, &analysis.Observations)
	_ = json.Unmarshal(snapshot.safetyConcerns, &analysis.SafetyConcerns)
	_ = json.Unmarshal(snapshot.additionalInfo, &analysis.AdditionalInfo)
	_ = json.Unmarshal(snapshot.measurements, &analysis.Measurements)
	_ = json.Unmarshal(snapshot.needsOnsiteMeasurement, &analysis.NeedsOnsiteMeasurement)
	_ = json.Unmarshal(snapshot.discrepancies, &analysis.Discrepancies)
	_ = json.Unmarshal(snapshot.extractedText, &analysis.ExtractedText)
	_ = json.Unmarshal(snapshot.suggestedSearchTerms, &analysis.SuggestedSearchTerms)
	return analysis
}


