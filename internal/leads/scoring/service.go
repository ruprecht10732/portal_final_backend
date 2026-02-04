package scoring

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

const (
	// scoreVersion tracks the scoring model for debugging and analysis.
	// Bump this when changing scoring logic significantly.
	scoreVersion = "2026-v2"

	// Base score - RAC_leads start at 50 and factors add/subtract from this.
	baseScore = 50.0

	// Maximum theoretical contribution from each factor category.
	// This ensures scores remain within 0-100 range.
	maxDemographicContribution = 35.0 // Ownership, wealth, income, household, children
	maxPropertyContribution    = 30.0 // Energy label, gas, electricity, building age
	maxBehavioralContribution  = 25.0 // Lead age, notes, photo, service status
	maxAIContribution          = 20.0 // AI urgency and quality
)

// serviceWeights defines how important each factor is for a specific service type.
// Values are multipliers (0.0-1.5) applied to base factor scores.
// Based on industry research for energy/home improvement lead qualification.
type serviceWeights struct {
	// Demographic factors
	ownership     float64 // Home ownership importance
	wealth        float64 // Financial capacity (median vermogen)
	income        float64 // Average income relevance
	incomeHigh    float64 // High income concentration
	incomeLow     float64 // Low income concentration (negative signal)
	household     float64 // Household size relevance
	children      float64 // Families with children
	stedelijkheid float64 // Urban/rural classification

	// Property/Energy factors
	energyLabel float64 // Poor energy label = opportunity
	gasUsage    float64 // High gas = heating opportunity
	electricity float64 // High electricity = solar opportunity
	buildingAge float64 // Older = more improvement potential
	wozValue    float64 // Property value indicator

	// Behavioral factors
	leadAge      float64 // Recency importance
	activity     float64 // Notes and engagement
	photo        float64 // Photo analysis available
	status       float64 // Service status indicator
	consumerNote float64 // Customer provided description
	source       float64 // Lead source quality
	assigned     float64 // Has assigned agent
	appointments float64 // Appointment activity
}

// defaultServiceWeights returns weights for services with unknown/generic type.
var defaultServiceWeights = serviceWeights{
	ownership:     1.0,
	wealth:        1.0,
	income:        1.0,
	incomeHigh:    0.8,
	incomeLow:     0.8,
	household:     1.0,
	children:      1.0,
	stedelijkheid: 0.5,
	energyLabel:   0.5,
	gasUsage:      0.5,
	electricity:   0.5,
	buildingAge:   0.8,
	wozValue:      0.8,
	leadAge:       1.0,
	activity:      1.0,
	photo:         1.0,
	status:        1.0,
	consumerNote:  1.0,
	source:        1.0,
	assigned:      1.0,
	appointments:  1.0,
}

// Service-type-specific weights based on industry research:
// - Energy services (solar, insulation, HVAC) prioritize energy data and ownership
// - Window replacements care about building age and energy performance
// - General services (plumbing, electrical, handyman) prioritize activity/engagement
var serviceWeightsMap = map[string]serviceWeights{
	// Solar: High electricity usage, ownership critical, wealth important for financing
	"solar": {
		ownership:     1.3,
		wealth:        1.2,
		income:        1.0,
		incomeHigh:    1.2, // High earners invest in solar
		incomeLow:     0.8,
		household:     0.8,
		children:      0.6,
		stedelijkheid: 0.6, // Rural better (more roof space, less shade)
		energyLabel:   0.8, // Less relevant - solar works regardless of label
		gasUsage:      0.2, // Solar doesn't replace gas
		electricity:   1.5, // Critical - high usage = high savings potential
		buildingAge:   0.6, // Less relevant - newer roofs work fine
		wozValue:      1.0,
		leadAge:       1.0,
		activity:      0.9,
		photo:         1.2, // Roof condition matters
		status:        1.0,
		consumerNote:  1.1, // Detailed requests show intent
		source:        1.0,
		assigned:      0.8, // Less important - solar is consultative
		appointments:  1.1, // Site survey critical
	},

	// Insulation: Poor energy labels are gold, high gas usage, older buildings
	"insulation": {
		ownership:     1.3,
		wealth:        1.0,
		income:        1.0,
		incomeHigh:    1.0,
		incomeLow:     0.9,
		household:     0.9,
		children:      0.8,
		stedelijkheid: 0.8, // Suburban houses often need more insulation
		energyLabel:   1.5, // Critical - E/F/G labels are prime targets
		gasUsage:      1.4, // High gas = poor insulation
		electricity:   0.5,
		buildingAge:   1.3, // Older = worse insulation typically
		wozValue:      0.9,
		leadAge:       1.0,
		activity:      1.0,
		photo:         1.1,
		status:        1.0,
		consumerNote:  1.2, // Problem description helps scope
		source:        1.0,
		assigned:      0.9,
		appointments:  1.0,
	},

	// HVAC/Heat pumps: High gas usage (replacing boilers), good insulation preferred
	"hvac": {
		ownership:     1.3,
		wealth:        1.3, // Heat pumps are expensive
		income:        1.1,
		incomeHigh:    1.3, // Premium investment
		incomeLow:     0.6, // Cost barrier
		household:     1.0,
		children:      0.8,
		stedelijkheid: 0.7, // Suburban/rural - more space for outdoor unit
		energyLabel:   1.2, // Better labels = ready for heat pump
		gasUsage:      1.4, // High gas = heating replacement opportunity
		electricity:   1.0,
		buildingAge:   0.8,
		wozValue:      1.1,
		leadAge:       1.0,
		activity:      1.0,
		photo:         1.0,
		status:        1.0,
		consumerNote:  1.1,
		source:        1.0,
		assigned:      0.9,
		appointments:  1.1, // Technical assessment needed
	},

	// Windows: Building age matters, energy performance relevant
	"windows": {
		ownership:     1.2,
		wealth:        1.0,
		income:        1.0,
		incomeHigh:    1.0,
		incomeLow:     0.8,
		household:     0.8,
		children:      0.7,
		stedelijkheid: 0.9, // Slightly less urban (apartments often shared)
		energyLabel:   1.0,
		gasUsage:      0.8, // Drafty windows = gas waste
		electricity:   0.4,
		buildingAge:   1.3, // Older buildings = older windows
		wozValue:      1.0,
		leadAge:       1.0,
		activity:      1.0,
		photo:         1.2, // Window condition visible in photos
		status:        1.0,
		consumerNote:  1.1,
		source:        1.0,
		assigned:      0.9,
		appointments:  1.0,
	},

	// Plumbing: Less demographic, more activity-focused
	"plumbing": {
		ownership:     0.8,
		wealth:        0.7,
		income:        0.8,
		incomeHigh:    0.6,
		incomeLow:     0.9, // Even low income needs plumbing fixes
		household:     1.1, // Larger households = more plumbing needs
		children:      1.0,
		stedelijkheid: 1.0, // Universal need
		energyLabel:   0.1,
		gasUsage:      0.3, // Gas for water heating
		electricity:   0.1,
		buildingAge:   1.0,
		wozValue:      0.7,
		leadAge:       1.2, // Urgency matters
		activity:      1.3, // Engagement indicates urgency
		photo:         1.3, // Photos show problem severity
		status:        1.1,
		consumerNote:  1.4, // Problem description crucial for plumbing
		source:        1.0,
		assigned:      1.2, // Quick response important
		appointments:  1.3, // Urgency - need quick appointment
	},

	// Electrical: Similar to plumbing, activity important
	"electrical": {
		ownership:     0.8,
		wealth:        0.8,
		income:        0.8,
		incomeHigh:    0.7,
		incomeLow:     0.9, // Safety-critical, even low income
		household:     0.9,
		children:      0.8,
		stedelijkheid: 1.0, // Universal need
		energyLabel:   0.2,
		gasUsage:      0.1,
		electricity:   0.8, // High usage might indicate electrical issues
		buildingAge:   1.1, // Older wiring needs updates
		wozValue:      0.8,
		leadAge:       1.2,
		activity:      1.3,
		photo:         1.2,
		status:        1.1,
		consumerNote:  1.3, // Safety context important
		source:        1.0,
		assigned:      1.2, // Quick response for safety
		appointments:  1.2,
	},

	// Carpentry: Building age and property value
	"carpentry": {
		ownership:     0.9,
		wealth:        0.9,
		income:        0.9,
		incomeHigh:    0.9,
		incomeLow:     0.7,
		household:     0.8,
		children:      0.8,
		stedelijkheid: 0.8, // Slightly suburban (more wood structures)
		energyLabel:   0.1,
		gasUsage:      0.1,
		electricity:   0.1,
		buildingAge:   1.0,
		wozValue:      1.0,
		leadAge:       1.1,
		activity:      1.2,
		photo:         1.2,
		status:        1.0,
		consumerNote:  1.2, // Project scope from description
		source:        1.0,
		assigned:      1.0,
		appointments:  1.0,
	},

	// Handyman: Most activity-focused, least demographic
	"handyman": {
		ownership:     0.6,
		wealth:        0.5,
		income:        0.6,
		incomeHigh:    0.4,
		incomeLow:     1.0, // Budget-conscious choose handyman
		household:     0.8,
		children:      0.9, // Families need more repairs
		stedelijkheid: 1.1, // Urban areas use handyman services more
		energyLabel:   0.0,
		gasUsage:      0.0,
		electricity:   0.0,
		buildingAge:   0.7,
		wozValue:      0.5,
		leadAge:       1.3, // Fresh RAC_leads convert best
		activity:      1.4, // Engagement is key
		photo:         1.3,
		status:        1.2,
		consumerNote:  1.3, // Task description important
		source:        1.1,
		assigned:      1.1,
		appointments:  1.2,
	},
}

// Result holds scoring output and factor details.
type Result struct {
	Score       int
	ScorePreAI  int
	FactorsJSON []byte
	Version     string
	UpdatedAt   time.Time
}

// Service computes lead scores.
type Service struct {
	repo repository.LeadsRepository
	log  *logger.Logger
}

// New creates a new scoring service.
func New(repo repository.LeadsRepository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Recalculate computes score for a lead and optionally includes AI adjustments.
func (s *Service) Recalculate(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, tenantID uuid.UUID, includeAI bool) (*Result, error) {
	lead, err := s.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, err
	}

	svc, err := s.resolveService(ctx, leadID, serviceID, tenantID)
	if err != nil {
		return nil, err
	}

	notes, err := s.repo.ListLeadNotes(ctx, leadID, tenantID)
	if err != nil {
		notes = nil
	}

	// Fetch appointment statistics for scoring
	apptStats, err := s.repo.GetLeadAppointmentStats(ctx, leadID, tenantID)
	if err != nil {
		apptStats = repository.LeadAppointmentStats{}
	}

	var photo *repository.PhotoAnalysis
	if svc != nil {
		if found, err := s.repo.GetLatestPhotoAnalysis(ctx, svc.ID, tenantID); err == nil {
			photo = &found
		}
	}

	var ai *repository.AIAnalysis
	if includeAI && svc != nil {
		if found, err := s.repo.GetLatestAIAnalysis(ctx, svc.ID, tenantID); err == nil {
			ai = &found
		}
	}

	// Get service type for weight selection
	serviceType := "default"
	if svc != nil && svc.ServiceType != "" {
		serviceType = strings.ToLower(svc.ServiceType)
	}

	now := time.Now().UTC()
	preAI, factors := s.computePreAIScore(lead, svc, notes, photo, apptStats, serviceType)
	finalScore, aiFactors := s.applyAIFactors(preAI, ai)
	if len(aiFactors) > 0 {
		for k, v := range aiFactors {
			factors[k] = v
		}
	}

	factorsJSON, err := json.Marshal(factors)
	if err != nil {
		if s.log != nil {
			s.log.Error("lead score factors marshal failed", "error", err)
		}
		factorsJSON = nil
	}

	return &Result{
		Score:       finalScore,
		ScorePreAI:  preAI,
		FactorsJSON: factorsJSON,
		Version:     scoreVersion,
		UpdatedAt:   now,
	}, nil
}

func (s *Service) resolveService(ctx context.Context, leadID uuid.UUID, serviceID *uuid.UUID, tenantID uuid.UUID) (*repository.LeadService, error) {
	if serviceID != nil {
		svc, err := s.repo.GetLeadServiceByID(ctx, *serviceID, tenantID)
		if err != nil {
			return nil, err
		}
		return &svc, nil
	}

	svc, err := s.repo.GetCurrentLeadService(ctx, leadID, tenantID)
	if err != nil {
		return nil, nil
	}
	return &svc, nil
}

// getServiceWeights returns the weight profile for a service type.
func getServiceWeights(serviceType string) serviceWeights {
	if w, ok := serviceWeightsMap[serviceType]; ok {
		return w
	}
	return defaultServiceWeights
}

func (s *Service) computePreAIScore(lead repository.Lead, svc *repository.LeadService, notes []repository.LeadNote, photo *repository.PhotoAnalysis, apptStats repository.LeadAppointmentStats, serviceType string) (int, map[string]float64) {
	score := baseScore
	factors := map[string]float64{}
	weights := getServiceWeights(serviceType)

	// Enrichment confidence applies to demographic/property factors
	confidence := 1.0
	if lead.LeadEnrichmentConfidence != nil {
		confidence = *lead.LeadEnrichmentConfidence
	}

	// ========== DEMOGRAPHIC FACTORS (max ~35 points) ==========
	// These factors describe WHO the lead is

	// Ownership: Homeowners can make decisions about improvements
	// Score: -5 to +10 based on % owner-occupied in area
	ownershipScore := s.scoreOwnership(lead) * weights.ownership * confidence
	score += s.addFactor(factors, "ownership", ownershipScore)

	// Wealth: Mediaan vermogen indicates financial capacity
	// Score: 0 to +12 based on wealth brackets
	wealthScore := s.scoreWealth(lead) * weights.wealth * confidence
	score += s.addFactor(factors, "wealth", wealthScore)

	// Income: Average household income
	// Score: 0 to +6 based on income level
	incomeScore := s.scoreIncome(lead) * weights.income * confidence
	score += s.addFactor(factors, "income", incomeScore)

	// Household size: Larger households typically have more needs
	// Score: 0 to +4
	householdScore := s.scoreHousehold(lead) * weights.household * confidence
	score += s.addFactor(factors, "household", householdScore)

	// Children: Families invest more in their homes
	// Score: 0 to +4
	childrenScore := s.scoreChildren(lead) * weights.children * confidence
	score += s.addFactor(factors, "children", childrenScore)

	// Stedelijkheid: Urban/rural affects service demand patterns
	// Score: -2 to +4
	stedelijkheidScore := s.scoreStedelijkheid(lead) * weights.stedelijkheid * confidence
	score += s.addFactor(factors, "stedelijkheid", stedelijkheidScore)

	// High income concentration: Premium service potential
	// Score: 0 to +5
	highIncomeScore := s.scoreHighIncome(lead) * weights.incomeHigh * confidence
	score += s.addFactor(factors, "income_high", highIncomeScore)

	// Low income concentration: Negative signal for premium services
	// Score: -4 to 0
	lowIncomeScore := s.scoreLowIncome(lead) * weights.incomeLow * confidence
	score += s.addFactor(factors, "income_low", lowIncomeScore)

	// ========== PROPERTY/ENERGY FACTORS (max ~30 points) ==========
	// These factors describe the PROPERTY and its energy profile

	// Energy label: Poor labels (E/F/G) = massive improvement opportunity
	// Score: -3 to +12
	energyLabelScore := s.scoreEnergyLabel(lead) * weights.energyLabel
	score += s.addFactor(factors, "energy_label", energyLabelScore)

	// Gas usage: High gas consumption indicates heating/insulation needs
	// Score: -4 to +8
	gasScore := s.scoreGas(lead) * weights.gasUsage * confidence
	score += s.addFactor(factors, "gas_usage", gasScore)

	// Electricity: High usage = solar opportunity
	// Score: 0 to +8
	electricityScore := s.scoreElectricity(lead) * weights.electricity * confidence
	score += s.addFactor(factors, "electricity", electricityScore)

	// Building age: Older buildings often need more work
	// Score: 0 to +6
	buildingAgeScore := s.scoreBuildingAge(lead) * weights.buildingAge
	score += s.addFactor(factors, "building_age", buildingAgeScore)

	// WOZ value: Property value indicates investment potential
	// Score: 0 to +4
	wozScore := s.scoreWOZ(lead) * weights.wozValue * confidence
	score += s.addFactor(factors, "woz_value", wozScore)

	// ========== BEHAVIORAL FACTORS (max ~25 points) ==========
	// These factors describe lead ENGAGEMENT and TIMING

	// Lead age: Fresh RAC_leads convert better (recency bias)
	// Score: -6 to +8
	leadAgeScore := s.scoreLeadAge(lead) * weights.leadAge
	score += s.addFactor(factors, "lead_age", leadAgeScore)

	// Service status: Where they are in the funnel
	// Score: -5 to +5
	statusScore := s.scoreServiceStatus(svc) * weights.status
	score += s.addFactor(factors, "service_status", statusScore)

	// Notes activity: Engagement level
	// Score: 0 to +6
	notesScore := s.scoreNotes(notes) * weights.activity
	score += s.addFactor(factors, "activity", notesScore)

	// Photo analysis: Shows serious intent
	// Score: 0 to +8
	photoScore := s.scorePhoto(photo) * weights.photo
	score += s.addFactor(factors, "photo", photoScore)

	// Consumer note: Customer's description of their need
	// Score: 0 to +8 based on length and content
	consumerNoteScore := s.scoreConsumerNote(svc) * weights.consumerNote
	score += s.addFactor(factors, "consumer_note", consumerNoteScore)

	// Lead source: Quality of acquisition channel
	// Score: -2 to +6
	sourceScore := s.scoreSource(lead, svc) * weights.source
	score += s.addFactor(factors, "source", sourceScore)

	// Assigned agent: Lead is being actively worked
	// Score: 0 to +4
	assignedScore := s.scoreAssigned(lead) * weights.assigned
	score += s.addFactor(factors, "assigned", assignedScore)

	// Appointments: Scheduled/completed appointments show commitment
	// Score: -3 to +10
	appointmentScore := s.scoreAppointments(apptStats) * weights.appointments
	score += s.addFactor(factors, "appointments", appointmentScore)

	return clampScore(score), factors
}

func (s *Service) applyAIFactors(preAI int, ai *repository.AIAnalysis) (int, map[string]float64) {
	if ai == nil {
		return preAI, map[string]float64{}
	}

	delta := 0.0
	factors := map[string]float64{}

	// AI urgency assessment: How time-sensitive is this lead?
	switch ai.UrgencyLevel {
	case "High":
		delta += 10
		factors["ai_urgency"] = 10
	case "Medium":
		delta += 4
		factors["ai_urgency"] = 4
	case "Low":
		delta -= 3
		factors["ai_urgency"] = -3
	}

	// AI quality assessment: How likely to convert?
	switch ai.LeadQuality {
	case "Urgent":
		delta += 12
		factors["ai_quality"] = 12
	case "High":
		delta += 7
		factors["ai_quality"] = 7
	case "Potential":
		delta += 2
		factors["ai_quality"] = 2
	case "Low":
		delta -= 8
		factors["ai_quality"] = -8
	case "Junk":
		delta -= 25
		factors["ai_quality"] = -25
	}

	return clampScore(float64(preAI) + delta), factors
}

func (s *Service) addFactor(factors map[string]float64, key string, value float64) float64 {
	if math.Abs(value) < 0.01 {
		return 0
	}
	// Round to 1 decimal place for cleaner factor display
	factors[key] = math.Round(value*10) / 10
	return value
}

// scoreOwnership evaluates home ownership percentage in the area.
// Homeowners are the primary decision-makers for home improvements.
// Thresholds based on CBS data: NL average is ~57% owner-occupied.
func (s *Service) scoreOwnership(lead repository.Lead) float64 {
	if lead.LeadEnrichmentKoopwoningenPct == nil {
		return 0
	}
	pct := *lead.LeadEnrichmentKoopwoningenPct
	switch {
	case pct >= 80:
		return 10 // Very high ownership area
	case pct >= 65:
		return 7 // Above average ownership
	case pct >= 50:
		return 4 // Average ownership
	case pct >= 35:
		return 0 // Below average
	default:
		return -5 // Very low ownership (rental dominated)
	}
}

// scoreWealth evaluates median household wealth (vermogen).
// Higher wealth = better ability to finance larger projects.
// Thresholds based on CBS wealth distribution data.
func (s *Service) scoreWealth(lead repository.Lead) float64 {
	if lead.LeadEnrichmentMediaanVermogenX1000 == nil {
		return 0
	}
	val := *lead.LeadEnrichmentMediaanVermogenX1000
	switch {
	case val >= 300:
		return 12 // Very wealthy area
	case val >= 150:
		return 8 // Above average wealth
	case val >= 75:
		return 5 // Average wealth
	case val >= 25:
		return 2 // Below average
	case val > 0:
		return 0 // Low wealth but positive
	default:
		return -2 // Negative median wealth (debt)
	}
}

// scoreIncome evaluates average household income.
// Income indicates short-term affordability.
func (s *Service) scoreIncome(lead repository.Lead) float64 {
	if lead.LeadEnrichmentGemInkomen == nil {
		return 0
	}
	val := *lead.LeadEnrichmentGemInkomen // in thousands EUR
	switch {
	case val >= 55:
		return 6 // High income area
	case val >= 40:
		return 4 // Above average
	case val >= 30:
		return 2 // Average
	default:
		return 0 // Below average
	}
}

// scoreGas evaluates average gas consumption.
// High gas usage indicates heating/insulation improvement potential.
// Based on CBS average of ~1200 m³/year for Dutch households.
func (s *Service) scoreGas(lead repository.Lead) float64 {
	if lead.LeadEnrichmentGemAardgasverbruik == nil {
		return 0
	}
	val := *lead.LeadEnrichmentGemAardgasverbruik
	switch {
	case val >= 2000:
		return 8 // Very high - major opportunity
	case val >= 1500:
		return 6 // High usage
	case val >= 1200:
		return 3 // Average usage
	case val >= 800:
		return 1 // Below average
	case val >= 400:
		return -2 // Low usage (likely already efficient)
	default:
		return -4 // Very low or no gas (electric/district heating)
	}
}

// scoreElectricity evaluates average electricity consumption.
// High electricity usage = good solar candidate.
// Based on CBS average of ~2700 kWh/year for Dutch households.
func (s *Service) scoreElectricity(lead repository.Lead) float64 {
	if lead.LeadEnrichmentGemElektriciteitsverbruik == nil {
		return 0
	}
	val := *lead.LeadEnrichmentGemElektriciteitsverbruik
	switch {
	case val >= 4500:
		return 8 // Very high - excellent solar candidate
	case val >= 3500:
		return 6 // High usage
	case val >= 2700:
		return 3 // Average
	case val >= 1800:
		return 1 // Below average
	default:
		return 0 // Low usage
	}
}

// scoreHousehold evaluates household size.
// Larger households have more needs and higher energy consumption.
func (s *Service) scoreHousehold(lead repository.Lead) float64 {
	if lead.LeadEnrichmentHuishoudenGrootte == nil {
		return 0
	}
	val := *lead.LeadEnrichmentHuishoudenGrootte
	switch {
	case val >= 3.0:
		return 4 // Large household
	case val >= 2.3:
		return 3 // Family-sized
	case val >= 1.8:
		return 1 // Couple
	default:
		return 0 // Single-person
	}
}

// scoreChildren evaluates percentage of households with children.
// Families tend to invest more in their homes for children's safety/comfort.
func (s *Service) scoreChildren(lead repository.Lead) float64 {
	if lead.LeadEnrichmentHuishoudensMetKinderenPct == nil {
		return 0
	}
	pct := *lead.LeadEnrichmentHuishoudensMetKinderenPct
	switch {
	case pct >= 45:
		return 4 // High family concentration
	case pct >= 30:
		return 2 // Above average
	default:
		return 0 // Lower family concentration
	}
}

// scoreStedelijkheid evaluates urban/rural classification.
// CBS stedelijkheid scale: 1 = very urban, 5 = rural
// Different services have different urban/rural demand patterns.
func (s *Service) scoreStedelijkheid(lead repository.Lead) float64 {
	if lead.LeadEnrichmentStedelijkheid == nil {
		return 0
	}
	val := *lead.LeadEnrichmentStedelijkheid
	switch val {
	case 1:
		return -2 // Very urban - limited for some services (solar roof space)
	case 2:
		return 0 // Urban
	case 3:
		return 2 // Suburban - often sweet spot
	case 4:
		return 3 // Semi-rural - good for energy services
	case 5:
		return 4 // Rural - more space, homeowners, DIY culture but also isolation
	default:
		return 0
	}
}

// scoreHighIncome evaluates percentage of high income households.
// High income concentration indicates premium service potential.
func (s *Service) scoreHighIncome(lead repository.Lead) float64 {
	if lead.LeadEnrichmentPctHoogInkomen == nil {
		return 0
	}
	pct := *lead.LeadEnrichmentPctHoogInkomen
	switch {
	case pct >= 30:
		return 5 // Very affluent area
	case pct >= 20:
		return 3 // Above average affluence
	case pct >= 10:
		return 1 // Some high earners
	default:
		return 0 // Low affluent area
	}
}

// scoreLowIncome evaluates percentage of low income households.
// High concentration of low income is a negative signal for premium services
// but neutral/positive for essential repairs (plumbing, electrical safety).
func (s *Service) scoreLowIncome(lead repository.Lead) float64 {
	if lead.LeadEnrichmentPctLaagInkomen == nil {
		return 0
	}
	pct := *lead.LeadEnrichmentPctLaagInkomen
	switch {
	case pct >= 40:
		return -4 // Very high low-income concentration
	case pct >= 25:
		return -2 // Above average low-income
	case pct >= 15:
		return -1 // Some low-income
	default:
		return 0 // Low concentration of low-income
	}
}

// scoreEnergyLabel evaluates the energy efficiency label.
// Poor labels (E/F/G) represent major improvement opportunities.
// This is one of the most predictive factors for energy services.
func (s *Service) scoreEnergyLabel(lead repository.Lead) float64 {
	delta := 0.0

	if lead.EnergyClass != nil {
		cls := strings.ToUpper(strings.TrimSpace(*lead.EnergyClass))
		switch cls {
		case "G":
			delta += 12 // Worst label = best opportunity
		case "F":
			delta += 10
		case "E":
			delta += 7
		case "D":
			delta += 4
		case "C":
			delta += 1
		case "B":
			delta -= 1
		case "A", "A+", "A++", "A+++", "A++++":
			delta -= 3 // Already efficient
		}
	}

	// Energy index provides more granular data
	if lead.EnergyIndex != nil {
		idx := *lead.EnergyIndex
		switch {
		case idx > 2.5:
			delta += 4 // Very poor efficiency
		case idx > 2.0:
			delta += 2
		case idx >= 1.4:
			delta += 1
		case idx < 0.8:
			delta -= 1 // Very efficient
		}
	}

	return delta
}

// scoreBuildingAge evaluates when the property was built.
// Older buildings typically need more improvements.
func (s *Service) scoreBuildingAge(lead repository.Lead) float64 {
	score := 0.0

	// Use EP-Online construction year if available
	if lead.EnergyBouwjaar != nil {
		year := *lead.EnergyBouwjaar
		switch {
		case year < 1960:
			score += 6 // Very old - significant improvement needs
		case year < 1980:
			score += 4 // Pre-insulation mandate
		case year < 1992:
			score += 2 // Before stricter building codes
		case year < 2010:
			score += 1
		default:
			score -= 1 // Modern building
		}
	}

	// Use CBS bouwjaar percentage as fallback/supplement
	if lead.LeadEnrichmentBouwjaarVanaf2000Pct != nil {
		pct := *lead.LeadEnrichmentBouwjaarVanaf2000Pct
		// Low percentage = mostly older buildings in area
		if pct <= 15 {
			score += 2
		} else if pct >= 70 {
			score -= 1
		}
	}

	return clampFloat(score, -2, 8)
}

// scoreWOZ evaluates property value as indicator of investment potential.
func (s *Service) scoreWOZ(lead repository.Lead) float64 {
	if lead.LeadEnrichmentWOZWaarde == nil {
		return 0
	}
	val := *lead.LeadEnrichmentWOZWaarde // in thousands EUR
	switch {
	case val >= 500:
		return 4 // High value property
	case val >= 350:
		return 3 // Above average
	case val >= 250:
		return 2 // Average
	case val >= 150:
		return 1 // Below average
	default:
		return 0 // Low value
	}
}

// scoreLeadAge evaluates how fresh the lead is.
// Fresh RAC_leads have higher conversion rates (recency bias).
func (s *Service) scoreLeadAge(lead repository.Lead) float64 {
	age := time.Since(lead.CreatedAt)
	hours := age.Hours()
	switch {
	case hours <= 24:
		return 8 // Same day - hot lead
	case hours <= 72:
		return 5 // Very fresh
	case hours <= 24*7:
		return 2 // Week old
	case hours <= 24*14:
		return 0 // Two weeks
	case hours <= 24*30:
		return -3 // Month old - cooling down
	default:
		return -6 // Stale lead
	}
}

// scoreServiceStatus evaluates where the lead is in the sales funnel.
func (s *Service) scoreServiceStatus(svc *repository.LeadService) float64 {
	if svc == nil {
		return 0
	}
	switch svc.Status {
	case "New":
		return 5 // Fresh opportunity
	case "Attempted_Contact":
		return 2 // In progress
	case "Contacted":
		return 1 // Engaged
	case "Scheduled":
		return -2 // Already scheduled, lower priority for scoring
	case "Completed", "Closed":
		return -5 // Done, shouldn't be prioritized
	default:
		return 0
	}
}

// scoreNotes evaluates engagement through note activity.
// More notes and recent activity indicates engaged prospect.
func (s *Service) scoreNotes(notes []repository.LeadNote) float64 {
	if len(notes) == 0 {
		return 0
	}

	score := 0.0

	// Note count indicates engagement depth
	switch {
	case len(notes) >= 5:
		score += 3 // High engagement
	case len(notes) >= 2:
		score += 2 // Some engagement
	default:
		score += 1 // Minimal engagement
	}

	// Recency of latest note
	latest := notes[0].CreatedAt
	for _, note := range notes {
		if note.CreatedAt.After(latest) {
			latest = note.CreatedAt
		}
	}

	hoursSince := time.Since(latest).Hours()
	switch {
	case hoursSince <= 24:
		score += 3 // Active today
	case hoursSince <= 72:
		score += 2 // Recent activity
	case hoursSince <= 24*7:
		score += 1 // Activity this week
	}

	return clampFloat(score, 0, 6)
}

// scorePhoto evaluates photo analysis data.
// Photos show serious intent and help qualify scope.
func (s *Service) scorePhoto(photo *repository.PhotoAnalysis) float64 {
	if photo == nil {
		return 0
	}

	score := 0.0

	// Having photos at all shows intent
	score += 2

	// Confidence in analysis
	switch photo.ConfidenceLevel {
	case "High":
		score += 2
	case "Medium":
		score += 1
	}

	// Scope indicates project size
	switch photo.ScopeAssessment {
	case "Large":
		score += 2
	case "Medium":
		score += 1
	}

	// Safety concerns indicate urgency
	if len(photo.SafetyConcerns) > 0 {
		score += 2
	}

	return clampFloat(score, 0, 8)
}

// scoreConsumerNote evaluates the customer's description of their need.
// Longer, more detailed descriptions indicate serious intent.
func (s *Service) scoreConsumerNote(svc *repository.LeadService) float64 {
	if svc == nil || svc.ConsumerNote == nil {
		return 0
	}

	note := strings.TrimSpace(*svc.ConsumerNote)
	length := len(note)

	// Length indicates effort/seriousness
	score := 0.0
	switch {
	case length == 0:
		return 0 // No note
	case length >= 300:
		score += 6 // Very detailed description
	case length >= 150:
		score += 4 // Good description
	case length >= 50:
		score += 2 // Basic description
	default:
		score += 1 // Minimal text
	}

	// Keywords indicating urgency
	lowerNote := strings.ToLower(note)
	urgentKeywords := []string{"urgent", "dringend", "snel", "asap", "lekkage", "kapot", "broken", "emergency", "noodgeval"}
	for _, kw := range urgentKeywords {
		if strings.Contains(lowerNote, kw) {
			score += 2
			break // Only count once
		}
	}

	return clampFloat(score, 0, 8)
}

// sourceScoreTable maps source keywords to their quality scores.
// Higher scores indicate better lead quality based on conversion rates.
var sourceScoreTable = []struct {
	keywords []string
	score    float64
}{
	// Best: Direct/referrals show high intent
	{[]string{"referral", "verwijzing"}, 6},
	{[]string{"direct", "inbound"}, 5},
	{[]string{"website", "organic"}, 4},
	// Good: Targeted campaigns
	{[]string{"email", "newsletter"}, 3},
	{[]string{"social", "facebook", "linkedin"}, 2},
	// Average: Paid acquisition
	{[]string{"google", "search"}, 2},
	{[]string{"partner", "affiliate"}, 1},
	// Lower: Mass market
	{[]string{"cold", "outbound"}, -1},
	{[]string{"purchased", "bought"}, -2},
}

// scoreSource evaluates lead acquisition channel quality.
// Different sources have different conversion rates.
func (s *Service) scoreSource(lead repository.Lead, svc *repository.LeadService) float64 {
	source := ""
	if svc != nil && svc.Source != nil {
		source = strings.ToLower(*svc.Source)
	} else if lead.Source != nil {
		source = strings.ToLower(*lead.Source)
	}

	if source == "" {
		return 0
	}

	for _, entry := range sourceScoreTable {
		if containsAny(source, entry.keywords) {
			return entry.score
		}
	}
	return 0 // Unknown source
}

// containsAny checks if s contains any of the keywords.
func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// scoreAssigned evaluates whether an agent is actively working the lead.
// Assigned RAC_leads have follow-up in progress, unassigned haven't started.
func (s *Service) scoreAssigned(lead repository.Lead) float64 {
	if lead.AssignedAgentID == nil {
		return 0 // Not assigned - neutral
	}
	// Having an assigned agent shows organization commitment
	return 4
}

// scoreAppointments evaluates appointment activity.
// Scheduled/completed appointments indicate serious buyer engagement.
func (s *Service) scoreAppointments(stats repository.LeadAppointmentStats) float64 {
	if stats.Total == 0 {
		return 0 // No appointments yet - neutral
	}

	score := 0.0

	// Has upcoming appointment - very engaged
	if stats.HasUpcoming {
		score += 4
	}

	// Completed appointments show progress
	score += float64(stats.Completed) * 2
	if stats.Completed >= 2 {
		score += 2 // Multiple visits = serious
	}

	// Scheduled but not completed yet
	score += float64(stats.Scheduled) * 1.5

	// Cancelled appointments are negative signal
	if stats.Cancelled > 0 {
		cancellationRate := float64(stats.Cancelled) / float64(stats.Total)
		if cancellationRate >= 0.5 {
			score -= 3 // High cancellation rate
		} else {
			score -= float64(stats.Cancelled)
		}
	}

	return clampFloat(score, -3, 10)
}

func clampScore(value float64) int {
	rounded := int(math.Round(value))
	if rounded < 0 {
		return 0
	}
	if rounded > 100 {
		return 100
	}
	return rounded
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
