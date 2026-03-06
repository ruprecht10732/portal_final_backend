package transport

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

func TestToAIAnalysisResponseIncludesConfidenceFields(t *testing.T) {
	score := 0.72
	analysis := repository.AIAnalysis{
		ID:                  uuid.New(),
		LeadID:              uuid.New(),
		OrganizationID:      uuid.New(),
		LeadServiceID:       uuid.New(),
		UrgencyLevel:        "Medium",
		LeadQuality:         "Potential",
		RecommendedAction:   "ScheduleSurvey",
		MissingInformation:  []string{"Foto van achterzijde"},
		CompositeConfidence: &score,
		ConfidenceBreakdown: map[string]float64{
			"llmCertainty":          0.60,
			"dataCompleteness":      0.85,
			"extractionReliability": 0.65,
			"businessValidation":    0.75,
		},
		RiskFlags:               []string{"missing_information"},
		PreferredContactChannel: "WhatsApp",
		SuggestedContactMessage: "Kunt u aanvullende foto's sturen?",
		Summary:                 "Intake gedeeltelijk compleet.",
		CreatedAt:               time.Now().UTC(),
	}

	res := ToAIAnalysisResponse(analysis)
	if res.CompositeConfidence == nil {
		t.Fatalf("expected compositeConfidence to be mapped")
	}
	if *res.CompositeConfidence != score {
		t.Fatalf("expected compositeConfidence %.2f, got %.2f", score, *res.CompositeConfidence)
	}
	if len(res.ConfidenceBreakdown) != 4 {
		t.Fatalf("expected 4 confidence breakdown entries, got %d", len(res.ConfidenceBreakdown))
	}
	if len(res.RiskFlags) != 1 || res.RiskFlags[0] != "missing_information" {
		t.Fatalf("expected risk flags to be mapped, got %+v", res.RiskFlags)
	}
}
