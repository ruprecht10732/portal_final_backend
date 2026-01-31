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
	LeadID              string              `json:"leadId"`
	LeadServiceID       string              `json:"leadServiceId"`       // The specific service this analysis is for
	UrgencyLevel        string              `json:"urgencyLevel"`        // High, Medium, Low
	UrgencyReason       string              `json:"urgencyReason"`       // Why this urgency level
	TalkingPoints       []string            `json:"talkingPoints"`       // Key points to discuss
	ObjectionHandling   []ObjectionResponse `json:"objectionHandling"`   // Likely objections with responses
	UpsellOpportunities []string            `json:"upsellOpportunities"` // Additional services to suggest
	Summary             string              `json:"summary"`             // Brief overall analysis
}

type SaveAnalysisOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ObjectionResponse matches the repository type
type ObjectionResponse struct {
	Objection string `json:"objection"`
	Response  string `json:"response"`
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

// AnalyzeResponse represents the result of an analysis request
type AnalyzeResponse struct {
	Status   string          `json:"status"` // "created", "no_change", "error"
	Message  string          `json:"message"`
	Analysis *AnalysisResult `json:"analysis,omitempty"`
}

// AnalysisResult represents the analysis returned to API consumers
type AnalysisResult struct {
	ID                  uuid.UUID           `json:"id"`
	LeadID              uuid.UUID           `json:"leadId"`
	LeadServiceID       *uuid.UUID          `json:"leadServiceId,omitempty"`
	UrgencyLevel        string              `json:"urgencyLevel"`
	UrgencyReason       *string             `json:"urgencyReason,omitempty"`
	TalkingPoints       []string            `json:"talkingPoints"`
	ObjectionHandling   []ObjectionResponse `json:"objectionHandling"`
	UpsellOpportunities []string            `json:"upsellOpportunities"`
	Summary             string              `json:"summary"`
	CreatedAt           string              `json:"createdAt"`
}
