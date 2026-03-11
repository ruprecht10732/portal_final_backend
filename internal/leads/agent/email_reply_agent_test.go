package agent

import (
	"strings"
	"testing"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"

	"github.com/google/uuid"
)

func TestBuildEmailReplyPromptWithoutLeadOrServiceContext(t *testing.T) {
	prompt := buildEmailReplyPrompt(ports.EmailReplyInput{
		CustomerEmail: "customer@example.com",
		CustomerName:  "Robin",
		Subject:       "Vraag over planning",
		MessageBody:   "Kunnen jullie volgende week langskomen?",
	}, emailReplyContext{}, "Behulpzaam en direct")

	checks := []string{
		"Lead ID: Niet opgegeven",
		"Service ID: Niet opgegeven",
		"Naam: Robin",
		"Onderwerp: Vraag over planning",
		"Bericht: Kunnen jullie volgende week langskomen?",
	}
	for _, expected := range checks {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got %s", expected, prompt)
		}
	}
}

func TestBuildEmailReplyPromptIncludesOverviewContext(t *testing.T) {
	firstName := "Robin"
	lastName := "Bos"
	note := "Klant wil graag snel duidelijkheid over de planning."
	prompt := buildEmailReplyPrompt(ports.EmailReplyInput{
		RequesterUserID: uuid.New(),
		Scenario:        ports.ReplySuggestionScenarioQuoteReminder,
		ScenarioNotes:   "Houd de toon ontspannen.",
		CustomerEmail:   "customer@example.com",
		CustomerName:    "Robin",
		Subject:         "Vraag over offerte",
		MessageBody:     "Welke offerte hebben we geaccepteerd?",
	}, emailReplyContext{
		requester:     &ports.ReplyUserProfile{Email: "robin@example.com", FirstName: &firstName, LastName: &lastName},
		acceptedQuote: &ports.PublicQuoteSummary{QuoteNumber: "OFF-2026-0001", Status: "Accepted", TotalCents: 123450, LineItems: []ports.PublicQuoteLineItemSummary{{Title: "Warmtepomp", Description: "Inclusief installatie", Quantity: "1", LineTotalCents: 123450}}},
		upcomingVisit: &ports.PublicAppointmentSummary{Title: "Inmeting", Status: "scheduled"},
		notes:         []repository.LeadNote{{Type: "important", Body: note}},
	}, "Behulpzaam en direct")

	checks := []string{
		"Requesting colleague",
		"Current date and time",
		"Selected reply scenario",
		"offerte opvolgen",
		"Houd de toon ontspannen.",
		"Gebruik dit om te bepalen of afspraken, deadlines en gebeurtenissen in het verleden of de toekomst liggen.",
		"Conversation intent hints",
		"Communication style hints",
		"Unknowns or missing context",
		"Naam: Robin Bos",
		"Accepted quote overview",
		"Offertenummer: OFF-2026-0001",
		"Inhoud:",
		"Warmtepomp",
		"Agenda overview",
		"Geplande afspraak:",
		"Belangrijk",
		note,
	}
	for _, expected := range checks {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got %s", expected, prompt)
		}
	}
}

func TestEmailReplySystemPromptUsesConfiguredTone(t *testing.T) {
	tone := "calm, practical, and reassuring"
	prompt := emailReplySystemPrompt(tone)

	if !strings.Contains(prompt, tone) {
		t.Fatalf("expected prompt to include configured tone %q, got %q", tone, prompt)
	}
	if !strings.Contains(prompt, "Do not include a subject line") {
		t.Fatalf("expected email-specific instruction in prompt, got %q", prompt)
	}
}

func TestEmailReplySystemPromptFallsBackToDefaultTone(t *testing.T) {
	defaultTone := ports.DefaultOrganizationAISettings().WhatsAppToneOfVoice
	prompt := emailReplySystemPrompt("   ")

	if !strings.Contains(prompt, defaultTone) {
		t.Fatalf("expected prompt to include default tone %q, got %q", defaultTone, prompt)
	}
}
