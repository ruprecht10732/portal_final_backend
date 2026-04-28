package agent

import (
	"testing"

	"portal_final_backend/internal/leads/repository"
)

func TestCalculateAnalysisConfidenceHighQualityCompleteLead(t *testing.T) {
	lead := repository.Lead{
		ConsumerFirstName:  "Jane",
		ConsumerLastName:   "Doe",
		ConsumerPhone:      "+31612345678",
		AddressStreet:      "Voorbeeldstraat",
		AddressHouseNumber: "12",
		AddressZipCode:     "1234AB",
		AddressCity:        "Amsterdam",
	}

	result := calculateAnalysisConfidence(lead, "High", "ScheduleSurvey", nil)

	if result.Score < 0.75 {
		t.Fatalf("expected high confidence for complete lead, got %.2f", result.Score)
	}
	if result.Score > 1.0 {
		t.Fatalf("confidence must be <= 1.0, got %.2f", result.Score)
	}
	if len(result.Breakdown) == 0 {
		t.Fatalf("expected non-empty confidence breakdown")
	}
}

func TestCalculateAnalysisConfidenceLowQualityMissingInfo(t *testing.T) {
	lead := repository.Lead{}

	result := calculateAnalysisConfidence(lead, "Low", "RequestInfo", []string{"Afmetingen ontbreken"})

	if result.Score > 0.50 {
		t.Fatalf("expected lower confidence for sparse lead, got %.2f", result.Score)
	}
	if len(result.RiskFlags) == 0 {
		t.Fatalf("expected risk flags for sparse lead")
	}
}

func TestClamp01Bounds(t *testing.T) {
	if got := clamp01(-1.2); got != 0 {
		t.Fatalf("expected clamp01(-1.2)=0, got %.2f", got)
	}
	if got := clamp01(1.3); got != 1 {
		t.Fatalf("expected clamp01(1.3)=1, got %.2f", got)
	}
	if got := clamp01(0.42); got != 0.42 {
		t.Fatalf("expected clamp01(0.42)=0.42, got %.2f", got)
	}
}
