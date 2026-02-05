package agent

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// TOOL INPUT/OUTPUT TYPES
// ============================================================================

// SaveAnalysisInput is the structured input for the SaveAnalysis tool
type SaveAnalysisInput struct {
	LeadID                  string   `json:"leadId"`
	LeadServiceID           string   `json:"leadServiceId"`           // The specific service this analysis is for
	UrgencyLevel            string   `json:"urgencyLevel"`            // High, Medium, Low
	UrgencyReason           string   `json:"urgencyReason"`           // Why this urgency level
	LeadQuality             string   `json:"leadQuality"`             // Junk, Low, Potential, High, Urgent
	RecommendedAction       string   `json:"recommendedAction"`       // Reject, RequestInfo, ScheduleSurvey, CallImmediately
	MissingInformation      []string `json:"missingInformation"`      // Missing critical info for triage
	PreferredContactChannel string   `json:"preferredContactChannel"` // WhatsApp, Email
	SuggestedContactMessage string   `json:"suggestedContactMessage"` // Message to send via chosen channel
	Summary                 string   `json:"summary"`                 // Brief overall analysis
}

type SaveAnalysisOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// UpdateLeadServiceTypeInput allows the agent to correct a mismatched service type.
type UpdateLeadServiceTypeInput struct {
	LeadID        string `json:"leadId"`
	LeadServiceID string `json:"leadServiceId"`
	ServiceType   string `json:"serviceType"` // Name or slug of an active service type
	Reason        string `json:"reason"`
}

type UpdateLeadServiceTypeOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DraftEmailInput for creating follow-up email drafts
type DraftEmailInput struct {
	LeadID      string   `json:"leadId"`
	Subject     string   `json:"subject"`
	Body        string   `json:"body"`
	Purpose     string   `json:"purpose"`               // "request_info", "confirm_appointment", "quote_followup", "general"
	MissingInfo []string `json:"missingInfo,omitempty"` // What information we need from customer
}

type DraftEmailOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	DraftID string `json:"draftId,omitempty"`
}

// EmailDraft represents a drafted email
type EmailDraft struct {
	ID          uuid.UUID
	LeadID      uuid.UUID
	Subject     string
	Body        string
	Purpose     string
	MissingInfo []string
	CreatedAt   time.Time
}

// GetPricingInput for service pricing lookup
type GetPricingInput struct {
	ServiceCategory string `json:"serviceCategory"` // "plumbing", "hvac", "electrical", "carpentry", "general"
	ServiceType     string `json:"serviceType"`     // specific service like "leaky_faucet", "boiler_repair", etc.
	Urgency         string `json:"urgency"`         // "normal", "same_day", "emergency"
}

type GetPricingOutput struct {
	PriceRangeLow    int      `json:"priceRangeLow"`
	PriceRangeHigh   int      `json:"priceRangeHigh"`
	TypicalDuration  string   `json:"typicalDuration"`
	IncludedServices []string `json:"includedServices"`
	Notes            string   `json:"notes"`
}

// SuggestSpecialistInput for recommending the right specialist
type SuggestSpecialistInput struct {
	ProblemDescription string `json:"problemDescription"`
	ServiceCategory    string `json:"serviceCategory,omitempty"` // optional hint
}

type SuggestSpecialistOutput struct {
	RecommendedSpecialist string   `json:"recommendedSpecialist"`
	Reason                string   `json:"reason"`
	AlternativeOptions    []string `json:"alternativeOptions,omitempty"`
	QuestionsToAsk        []string `json:"questionsToAsk"`
}

// UpdatePipelineStageInput updates the pipeline stage for the lead service.
type UpdatePipelineStageInput struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

type UpdatePipelineStageOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// FindMatchingPartnersInput searches for partner matches.
type FindMatchingPartnersInput struct {
	ServiceType string `json:"serviceType"`
	ZipCode     string `json:"zipCode"`
	RadiusKm    int    `json:"radiusKm"`
}

type PartnerMatch struct {
	PartnerID    string  `json:"partnerId"`
	BusinessName string  `json:"businessName"`
	Email        string  `json:"email"`
	DistanceKm   float64 `json:"distanceKm"`
}

type FindMatchingPartnersOutput struct {
	Matches []PartnerMatch `json:"matches"`
}

// SaveEstimationInput stores scope and price range in the timeline.
type SaveEstimationInput struct {
	Scope      string `json:"scope"`
	PriceRange string `json:"priceRange"`
	Notes      string `json:"notes"`
	Summary    string `json:"summary"`
}

type SaveEstimationOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// AnalyzeResponse represents the result of an analysis request
type AnalyzeResponse struct {
	Status   string          `json:"status"` // "created", "no_change", "error"
	Message  string          `json:"message"`
	Analysis *AnalysisResult `json:"analysis,omitempty"`
}

// AnalysisResult represents the analysis returned to API consumers
type AnalysisResult struct {
	ID                      uuid.UUID `json:"id"`
	LeadID                  uuid.UUID `json:"leadId"`
	LeadServiceID           uuid.UUID `json:"leadServiceId"`
	UrgencyLevel            string    `json:"urgencyLevel"`
	UrgencyReason           *string   `json:"urgencyReason,omitempty"`
	LeadQuality             string    `json:"leadQuality"`
	RecommendedAction       string    `json:"recommendedAction"`
	MissingInformation      []string  `json:"missingInformation"`
	PreferredContactChannel string    `json:"preferredContactChannel"`
	SuggestedContactMessage string    `json:"suggestedContactMessage"`
	Summary                 string    `json:"summary"`
	CreatedAt               string    `json:"createdAt"`
}
