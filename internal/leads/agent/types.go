package agent

import (
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

// UpdateLeadDetailsInput allows the agent to correct lead details with high confidence.
type UpdateLeadDetailsInput struct {
	LeadID       string   `json:"leadId"`
	FirstName    *string  `json:"firstName,omitempty"`
	LastName     *string  `json:"lastName,omitempty"`
	Phone        *string  `json:"phone,omitempty"`
	Email        *string  `json:"email,omitempty"`
	ConsumerRole *string  `json:"consumerRole,omitempty"`
	Street       *string  `json:"street,omitempty"`
	HouseNumber  *string  `json:"houseNumber,omitempty"`
	ZipCode      *string  `json:"zipCode,omitempty"`
	City         *string  `json:"city,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`
}

type UpdateLeadDetailsOutput struct {
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	UpdatedFields []string `json:"updatedFields,omitempty"`
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

// SearchProductMaterialsInput searches the product catalog for matching materials.
type SearchProductMaterialsInput struct {
	Query      string   `json:"query"`                // Natural language description of materials needed
	Limit      int      `json:"limit"`                // Max number of results (default 5)
	UseCatalog *bool    `json:"useCatalog,omitempty"` // Prefer catalog collection when true
	MinScore   *float64 `json:"minScore,omitempty"`   // Minimum similarity score (0-1, default 0.35)
}

// ProductResult represents a product found in the catalog.
type ProductResult struct {
	ID          string   `json:"id,omitempty"` // Catalog product UUID (present for catalog items)
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`         // "service", "digital_service", "product", or "material"
	PriceEuros  float64  `json:"priceEuros"`     // Unit price in euros (e.g., 7.93 = EUR 7.93)
	PriceCents  int64    `json:"priceCents"`     // Unit price in euro-cents, ready for unitPriceCents (e.g., 793)
	Unit        string   `json:"unit,omitempty"` // e.g., "per m2", "per stuk", "per m1"
	LaborTime   string   `json:"laborTime,omitempty"`
	VatRateBps  int      `json:"vatRateBps,omitempty"` // VAT rate in basis points (e.g. 2100 = 21%)
	Materials   []string `json:"materials,omitempty"`  // Included materials (human-readable names)
	Category    string   `json:"category,omitempty"`   // Product category path (e.g., "Douglas hout > balken")
	SourceURL   string   `json:"sourceUrl,omitempty"`  // Reference URL (fallback/scraped products only)
	Score       float64  `json:"score"`                // Similarity score
}

// SearchProductMaterialsOutput contains the search results.
type SearchProductMaterialsOutput struct {
	Products []ProductResult `json:"products"`
	Message  string          `json:"message"`
}

// CalculatorInput is the input for the general-purpose Calculator tool.
// The LLM MUST use this for ANY arithmetic to avoid mental-math errors.
type CalculatorInput struct {
	Operation string  `json:"operation"` // "add", "subtract", "multiply", "divide", "ceil_divide", "ceil", "floor", "round", "percentage"
	A         float64 `json:"a"`         // First operand (always required)
	B         float64 `json:"b"`         // Second operand (required for binary ops; for "round" = decimal places)
}

// CalculatorOutput returns the exact arithmetic result.
type CalculatorOutput struct {
	Result     float64 `json:"result"`     // The computed value
	Expression string  `json:"expression"` // Human-readable expression, e.g. "2 × 1.5 = 3"
}

// CalculateEstimateInput performs deterministic totals for materials and labor.
type CalculateEstimateInput struct {
	MaterialItems  []EstimateItem `json:"materialItems"`
	LaborHoursLow  float64        `json:"laborHoursLow"`
	LaborHoursHigh float64        `json:"laborHoursHigh"`
	HourlyRateLow  float64        `json:"hourlyRateLow"`
	HourlyRateHigh float64        `json:"hourlyRateHigh"`
	ExtraCosts     float64        `json:"extraCosts,omitempty"`
}

type EstimateItem struct {
	Label     string  `json:"label"`
	UnitPrice float64 `json:"unitPrice"`
	Quantity  float64 `json:"quantity"`
}

type CalculateEstimateOutput struct {
	MaterialSubtotal  float64 `json:"materialSubtotal"`
	LaborSubtotalLow  float64 `json:"laborSubtotalLow"`
	LaborSubtotalHigh float64 `json:"laborSubtotalHigh"`
	TotalLow          float64 `json:"totalLow"`
	TotalHigh         float64 `json:"totalHigh"`
	AppliedExtraCosts float64 `json:"appliedExtraCosts"`
}

// DraftQuoteItem represents a single line item for the DraftQuote tool.
type DraftQuoteItem struct {
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"` // e.g. "3", "1"
	UnitPriceCents   int64   `json:"unitPriceCents"`
	TaxRateBps       int     `json:"taxRateBps"`
	IsOptional       bool    `json:"isOptional,omitempty"`
	CatalogProductID *string `json:"catalogProductId,omitempty"` // UUID string from search results
}

// DraftQuoteInput is the structured input for the DraftQuote tool.
type DraftQuoteInput struct {
	Notes string           `json:"notes"`
	Items []DraftQuoteItem `json:"items"`
}

// DraftQuoteOutput is the result of the DraftQuote tool.
type DraftQuoteOutput struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	QuoteID     string `json:"quoteId,omitempty"`
	QuoteNumber string `json:"quoteNumber,omitempty"`
	ItemCount   int    `json:"itemCount,omitempty"`
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
