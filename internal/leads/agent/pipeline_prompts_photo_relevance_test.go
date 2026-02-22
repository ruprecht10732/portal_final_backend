package agent

import (
	"strings"
	"testing"

	"portal_final_backend/internal/leads/repository"
)

func TestIsPhotoAnalysisLikelyIrrelevant_MismatchInSummary(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Bijgevoegde foto's tonen niet de betreffende trapopening.",
		ConfidenceLevel: "Low",
	}
	if !isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected mismatch summary to be flagged as irrelevant")
	}
}

func TestIsPhotoAnalysisLikelyIrrelevant_LowConfidenceWithDiscrepancy(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont een andere ruimte.",
		ConfidenceLevel: "Low",
		Discrepancies:   []string{"Aanvraag en foto komen niet overeen"},
	}
	if !isPhotoAnalysisLikelyIrrelevant(analysis) {
		t.Fatalf("expected low-confidence discrepancy to be flagged as irrelevant")
	}
}

func TestBuildGatekeeperPhotoSummary_IncludesDetailsWhenIrrelevant(t *testing.T) {
	analysis := &repository.PhotoAnalysis{
		Summary:         "Foto toont plafond, niet trapopening.",
		ConfidenceLevel: "Low",
		Discrepancies:   []string{"Aanvraag en foto komen niet overeen"},
		Observations:    []string{"Woonruimte met plafondconstructie"},
	}

	result := buildGatekeeperPhotoSummary(analysis, "Deur plaatsen")
	if !strings.Contains(result, "Photo relevance: low for service type &apos;Deur plaatsen&apos;") {
		t.Fatalf("expected mismatch warning in summary")
	}
	if !strings.Contains(result, "Summary: Foto toont plafond, niet trapopening.") {
		t.Fatalf("expected original analysis summary details to be included")
	}
	if !strings.Contains(result, "Discrepancies") {
		t.Fatalf("expected discrepancy details to be included")
	}
}

