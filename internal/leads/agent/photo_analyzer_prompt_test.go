package agent

import (
	"strings"
	"testing"

	"portal_final_backend/internal/orchestration"

	"github.com/google/uuid"
)

func TestBuildPhotoAnalysisPromptAvoidsReferenceObjectMeasurements(t *testing.T) {
	prompt := buildPhotoAnalysisPrompt(uuid.New(), uuid.New(), 2, "", "Kozijn vervangen", "Breedte opening vereist", nil)

	checks := []string{
		"Gebruik foto's NIET als betrouwbare bron voor absolute meters",
		"Gebruik geen speculatieve referentie-objecten zoals deuren, stopcontacten of tegels om absolute afmetingen af te leiden.",
		"Standaard componenten en configuraties",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected photo analysis prompt to contain %q", token)
		}
	}

	forbidden := []string{
		"Gebruik referentie-objecten",
		"deuren ~2.1m",
	}

	for _, token := range forbidden {
		if strings.Contains(prompt, token) {
			t.Fatalf("expected photo analysis prompt to omit %q", token)
		}
	}
}

func TestGetPhotoAnalyzerPromptRequiresOnsiteVerificationForUncertainDimensions(t *testing.T) {
	workspace, err := orchestration.LoadAgentWorkspace(photoAnalyzerWorkspaceName)
	if err != nil {
		t.Fatalf("LoadAgentWorkspace returned error: %v", err)
	}
	prompt := workspace.Instruction

	checks := []string{
		"Behandel normale 2D foto's NIET als betrouwbare bron voor absolute maatvoering",
		"Leg alleen metingen vast als de waarde expliciet zichtbaar, gelabeld of via OCR verifieerbaar is.",
		"Als exacte maatvoering nodig is",
		"FlagOnsiteMeasurement",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected analyzer system prompt to contain %q", token)
		}
	}
}
