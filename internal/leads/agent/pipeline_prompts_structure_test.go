package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const toolOrderMandatoryHeader = "=== TOOL ORDER (MANDATORY) ==="

func testPromptFixtures() (repository.Lead, repository.LeadService, []repository.LeadNote, []repository.Attachment, *repository.PhotoAnalysis) {
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	email := "jane@example.com"
	noteText := "Klant wil teruggebeld worden na 18:00"

	lead := repository.Lead{
		ID:                 uuid.New(),
		ConsumerFirstName:  "Jane",
		ConsumerLastName:   "Doe",
		ConsumerPhone:      "+31612345678",
		ConsumerEmail:      &email,
		ConsumerRole:       "Owner",
		AddressStreet:      "Voorbeeldstraat",
		AddressHouseNumber: "12",
		AddressZipCode:     "1234AB",
		AddressCity:        "Amsterdam",
		CreatedAt:          now,
	}

	service := repository.LeadService{
		ID:            uuid.New(),
		ServiceType:   "Kozijn vervangen",
		PipelineStage: "Triage",
		ConsumerNote:  &noteText,
	}

	notes := []repository.LeadNote{{
		Type:      "call",
		Body:      "Bel na werktijd terug",
		CreatedAt: now,
	}}

	attachments := []repository.Attachment{{
		FileName: "foto-voordeur.jpg",
	}}

	photo := &repository.PhotoAnalysis{
		Summary:         "Voordeur met zichtbare slijtage",
		ConfidenceLevel: "High",
	}

	return lead, service, notes, attachments, photo
}

func TestBuildGatekeeperPromptUsesExecutionContractAndOrder(t *testing.T) {
	lead, service, notes, attachments, photo := testPromptFixtures()
	prompt := buildGatekeeperPrompt(lead, service, notes, "Meetgegevens vereist", attachments, photo)

	checks := []string{
		"=== EXECUTION CONTRACT ===",
		"=== EXECUTION ORDER ===",
		"1. UpdateLeadDetails",
		"2. UpdateLeadServiceType",
		"3. SaveAnalysis",
		"4. UpdatePipelineStage",
		"=== DECISION TABLE ===",
		"=== SELF-CHECK BEFORE FINAL TOOL CALL ===",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected gatekeeper prompt to contain %q", token)
		}
	}
}

func TestBuildEstimatorPromptUsesCanonicalToolOrder(t *testing.T) {
	lead, service, notes, _, photo := testPromptFixtures()
	prompt := buildEstimatorPrompt(lead, service, notes, photo, "Gebruik standaard afwerking")

	checks := []string{
		"=== EXECUTION PRIORITY ===",
		toolOrderMandatoryHeader,
		"1. ListCatalogGaps (once)",
		"2. SearchProductMaterials (repeat as needed)",
		"3. Calculator",
		"4. CalculateEstimate",
		"5. DraftQuote",
		"6. SaveEstimation",
		"7. UpdatePipelineStage",
		"=== MATH MODEL ===",
		"=== PRODUCT DECISION TABLE ===",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected estimator prompt to contain %q", token)
		}
	}
}

func TestBuildDispatcherPromptUsesScoringModel(t *testing.T) {
	lead, service, _, _, _ := testPromptFixtures()
	prompt := buildDispatcherPrompt(lead, service, 25, nil)

	checks := []string{
		toolOrderMandatoryHeader,
		"1. FindMatchingPartners",
		"2. CreatePartnerOffer",
		"3. UpdatePipelineStage",
		"=== PARTNER SCORING ===",
		"score = (-2 * rejectedOffers30d) + (-1 * openOffers30d) + (-0.2 * distanceKm)",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected dispatcher prompt to contain %q", token)
		}
	}
}

func TestBuildQuoteGeneratePromptUsesToolScopeAndSharedRules(t *testing.T) {
	lead, service, notes, _, _ := testPromptFixtures()
	prompt := buildQuoteGeneratePrompt(lead, service, notes, "Vervang voordeur inclusief scharnieren", "Let op isolatie")

	checks := []string{
		"=== TOOL SCOPE (MANDATORY) ===",
		"You MAY call only: SearchProductMaterials, Calculator, DraftQuote.",
		toolOrderMandatoryHeader,
		"=== PRODUCT DECISION TABLE ===",
		"=== SEARCH STRATEGY (MAX 3 PER MATERIAL) ===",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected quote generator prompt to contain %q", token)
		}
	}
}
