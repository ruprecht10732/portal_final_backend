package agent

import (
	"strings"
	"testing"

	"portal_final_backend/internal/leads/repository"
)

func TestIsPhotoAnalysisLikelyIrrelevantExplicitFalse(t *testing.T) {
	f := false
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont een mooie tuin.",
		ConfidenceLevel: "High",
		IsRelevant:      &f,
	}
	if !isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected explicit IsRelevant=false to be flagged as irrelevant")
	}
}

func TestIsPhotoAnalysisLikelyIrrelevantExplicitTrue(t *testing.T) {
	tr := true
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont een mooie tuin.",
		ConfidenceLevel: "Low",
		Discrepancies:   []string{"Aanvraag en foto komen niet overeen"},
		IsRelevant:      &tr,
	}
	if isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected explicit IsRelevant=true to override text heuristics")
	}
}

func TestIsPhotoAnalysisLikelyIrrelevantMismatchInSummary(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Bijgevoegde foto's tonen niet de betreffende trapopening.",
		ConfidenceLevel: "Low",
	}
	if !isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected mismatch summary to be flagged as irrelevant")
	}
}

func TestIsPhotoAnalysisLikelyIrrelevantLowConfidenceWithDiscrepancy(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont een andere ruimte.",
		ConfidenceLevel: "Low",
		Discrepancies:   []string{"Aanvraag en foto komen niet overeen"},
	}
	if !isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected low-confidence discrepancy to be flagged as irrelevant")
	}
}

func TestBuildGatekeeperPhotoSummaryIncludesDetailsWhenIrrelevant(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont plafond, niet trapopening.",
		ConfidenceLevel: "Low",
		Discrepancies:   []string{"Aanvraag en foto komen niet overeen"},
		Observations:    []string{"Woonruimte met plafondconstructie"},
	}

	result := buildGatekeeperPhotoSummary(analysis, "Deur plaatsen")
	if !strings.Contains(result, "Photo relevance: low for service type 'Deur plaatsen'") {
		t.Fatalf("expected mismatch warning in summary")
	}
	if !strings.Contains(result, "Summary: Foto toont plafond, niet trapopening.") {
		t.Fatalf("expected original analysis summary details to be included")
	}
	if !strings.Contains(result, "Discrepancies") {
		t.Fatalf("expected discrepancy details to be included")
	}
}

func TestBuildPhotoSummaryContentAddsMeasurementGuardrail(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:                "Foto toont kozijn en glaslatten.",
		ConfidenceLevel:        "Medium",
		Measurements:           []repository.Measurement{{Description: "breedte kozijn", Value: 1.2, Unit: "m", Type: "dimension", Confidence: "Low"}},
		NeedsOnsiteMeasurement: []string{"Exacte dagmaat niet verifieerbaar door perspectief"},
	}

	result := buildPhotoSummaryContent(analysis)
	if !strings.Contains(result, "Measurement guardrail: Treat photo-derived dimensions as advisory only unless they are explicitly visible, labeled, or OCR-backed.") {
		t.Fatalf("expected measurement guardrail in photo summary")
	}
	if !strings.Contains(result, "Needs on-site measurement: Exacte dagmaat niet verifieerbaar door perspectief") {
		t.Fatalf("expected on-site measurement reason in photo summary")
	}
}
