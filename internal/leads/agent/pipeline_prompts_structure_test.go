package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const toolOrderMandatoryHeader = "=== TOOL ORDER (MANDATORY) ==="
const gatekeeperIntakeRequirement = "Meetgegevens vereist"
const expectedGatekeeperPromptContainsFmt = "expected gatekeeper prompt to contain %q"

func testPromptFixtures() (repository.Lead, repository.LeadService, []repository.LeadNote, *repository.AppointmentVisitReport, []repository.Attachment, *repository.PhotoAnalysis) {
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	email := "jane@example.com"
	noteText := "Klant wil teruggebeld worden na 18:00"
	measurementText := "Breedte 830 mm, hoogte 1525 mm"
	visitNotes := "Trap dichtmaken met stootborden en kastconstructie"

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

	visitReport := &repository.AppointmentVisitReport{
		AppointmentID: uuid.New(),
		Measurements:  &measurementText,
		Notes:         &visitNotes,
	}

	attachments := []repository.Attachment{{
		FileName: "foto-voordeur.jpg",
	}}

	photo := &repository.PhotoAnalysis{
		Summary:         "Voordeur met zichtbare slijtage",
		ConfidenceLevel: "High",
	}

	return lead, service, notes, visitReport, attachments, photo
}

func TestBuildGatekeeperPromptUsesExecutionContractAndOrder(t *testing.T) {
	lead, service, notes, visitReport, attachments, photo := testPromptFixtures()
	prompt := buildGatekeeperPrompt(gatekeeperPromptInput{
		lead:          lead,
		service:       service,
		notes:         notes,
		visitReport:   visitReport,
		intakeContext: gatekeeperIntakeRequirement,
		attachments:   attachments,
		photoAnalysis: photo,
	})

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
			t.Fatalf(expectedGatekeeperPromptContainsFmt, token)
		}
	}
}

func TestBuildGatekeeperPromptIncludesVisitReportEvidence(t *testing.T) {
	lead, service, notes, visitReport, attachments, photo := testPromptFixtures()
	prompt := buildGatekeeperPrompt(gatekeeperPromptInput{
		lead:          lead,
		service:       service,
		notes:         notes,
		visitReport:   visitReport,
		intakeContext: gatekeeperIntakeRequirement,
		attachments:   attachments,
		photoAnalysis: photo,
	})

	checks := []string{
		"Visit Report (latest appointment):",
		"Breedte 830 mm, hoogte 1525 mm",
		"Trap dichtmaken met stootborden en kastconstructie",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf(expectedGatekeeperPromptContainsFmt, token)
		}
	}
}

func TestBuildGatekeeperPromptIncludesPreviousEstimatorBlockers(t *testing.T) {
	lead, service, notes, visitReport, attachments, photo := testPromptFixtures()
	confidence := 0.31
	priorAnalysis := &repository.AIAnalysis{
		RecommendedAction:   "RequestInfo",
		MissingInformation:  []string{"dagmaat van het kozijn", "foto van de binnenzijde"},
		RiskFlags:           []string{"missing_dimensions"},
		CompositeConfidence: &confidence,
		Summary:             "Estimator kon nog geen scope afronden door ontbrekende maatvoering.",
	}

	prompt := buildGatekeeperPrompt(gatekeeperPromptInput{
		lead:          lead,
		service:       service,
		notes:         notes,
		visitReport:   visitReport,
		intakeContext: gatekeeperIntakeRequirement,
		attachments:   attachments,
		photoAnalysis: photo,
		priorAnalysis: priorAnalysis,
	})

	checks := []string{
		"Previous Estimator Blockers:",
		"Laatste aanbevolen actie: RequestInfo",
		"Eerder ontbrekende intakegegevens: dagmaat van het kozijn, foto van de binnenzijde",
		"Risicosignalen: missing_dimensions",
		"Confidence vorige analyse: 0.31",
		"Estimator previously blocked this lead for missing information",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf(expectedGatekeeperPromptContainsFmt, token)
		}
	}
}

func TestGatekeeperPromptKeepsEstimatorBlockersAfterCustomerReplyWithoutMeasurements(t *testing.T) {
	lead, service, _, visitReport, attachments, photo := testPromptFixtures()
	service.PipelineStage = "Nurturing"
	notes := []repository.LeadNote{{
		Type:      "message",
		Body:      "Ik heb geen meetlint.",
		CreatedAt: lead.CreatedAt.Add(2 * time.Hour),
	}}
	priorAnalysis := &repository.AIAnalysis{
		RecommendedAction:  "RequestInfo",
		MissingInformation: []string{"dagmaat van het kozijn", "hoogte van de opening"},
		Summary:            "Estimator heeft exacte maatvoering nodig voordat de scope compleet is.",
	}

	prompt := buildGatekeeperPrompt(gatekeeperPromptInput{
		lead:          lead,
		service:       service,
		notes:         notes,
		visitReport:   visitReport,
		intakeContext: gatekeeperIntakeRequirement,
		attachments:   attachments,
		photoAnalysis: photo,
		priorAnalysis: priorAnalysis,
	})

	checks := []string{
		"Ik heb geen meetlint.",
		"Eerder ontbrekende intakegegevens: dagmaat van het kozijn, hoogte van de opening",
		"you MUST NOT move to Estimation until that exact information is explicitly present in trusted context",
	}

	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf(expectedGatekeeperPromptContainsFmt, token)
		}
	}
}

func TestBuildEstimatorPromptUsesCanonicalToolOrder(t *testing.T) {
	lead, service, notes, _, _, photo := testPromptFixtures()
	prompt := buildQuoteBuilderPrompt(lead, service, notes, photo, "Gebruik standaard afwerking", nil)

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
	lead, service, _, _, _, _ := testPromptFixtures()
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
	lead, service, notes, _, _, _ := testPromptFixtures()
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
